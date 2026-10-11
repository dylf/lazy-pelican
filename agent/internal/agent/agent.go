// Package agent turns "someone is trying to join" signals into panel start requests, with safety limits.
package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/detector"
)

// Panel is the slice of the panel API the agent needs.
type Panel interface {
	State(ctx context.Context, id string) (string, error)
	Start(ctx context.Context, id string) error
}

type Server struct {
	Name string
	ID   string
}

type Options struct {
	Servers         []Server      // initial set; SetPlan replaces it
	Cooldown        time.Duration // after a start, ignore triggers for this long
	MaxWakesPerHour int
	DryRun          bool
	Now             func() time.Time
	Log             *slog.Logger

	// OnWake, if set, is called (on its own goroutine) after the agent has started a server.
	OnWake func(s Server, at time.Time)
}

// ServerStats is what the agent has seen for one watched server, for heartbeats.
type ServerStats struct {
	Server        Server
	LastDetection time.Time // zero if never
	LastWake      time.Time // zero if never
}

// Outcome says what Handle did, mainly so tests can assert on it.
type Outcome string

const (
	Started     Outcome = "started"
	DryRun      Outcome = "dry-run"
	Quiet       Outcome = "quiet"        // inside a cooldown or back-off, panel not asked
	NotOffline  Outcome = "not-offline"  // already starting, running or stopping
	RateLimited Outcome = "rate-limited" // hit max wakes per hour
	Failed      Outcome = "error"
	Unknown     Outcome = "unknown-server"
)

// How long to leave a server alone after learning it isn't startable, or after the panel errored. Triggers keep
// arriving while players are connected to a running server, and we must not hammer the panel for each one.
const (
	notOfflineBackoff = 30 * time.Second
	errorBackoff      = 15 * time.Second
	rateLimitBackoff  = time.Minute
	suspendedBackoff  = 5 * time.Minute
)

type Agent struct {
	opts  Options
	panel Panel

	// Only touched by Handle, which Run serialises.
	quietUntil map[string]time.Time
	wakes      map[string][]time.Time

	// Shared with SetPlan, NoteDetection and Stats, which run on other goroutines.
	mu            sync.Mutex
	order         []Server
	servers       map[string]Server
	cooldown      time.Duration
	maxWakes      int
	lastDetection map[string]time.Time
	lastWake      map[string]time.Time
}

func New(p Panel, o Options) *Agent {
	if o.Now == nil {
		o.Now = time.Now
	}

	if o.Log == nil {
		o.Log = slog.Default()
	}

	a := &Agent{
		opts: o, panel: p,
		quietUntil: map[string]time.Time{}, wakes: map[string][]time.Time{},
		lastDetection: map[string]time.Time{}, lastWake: map[string]time.Time{},
	}
	a.SetPlan(o.Servers, o.Cooldown, o.MaxWakesPerHour)

	return a
}

// SetPlan replaces the servers and limits. Cooldowns already running are kept, so a config refresh can't be used
// (or tripped by accident) to get around them.
func (a *Agent) SetPlan(servers []Server, cooldown time.Duration, maxWakesPerHour int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.order = append([]Server(nil), servers...)
	a.servers = make(map[string]Server, len(servers))

	for _, s := range servers {
		a.servers[s.Name] = s
	}

	a.cooldown, a.maxWakes = cooldown, maxWakesPerHour
}

// NoteDetection records when a join attempt was last seen for a server.
func (a *Agent) NoteDetection(name string, at time.Time) {
	a.mu.Lock()
	a.lastDetection[name] = at
	a.mu.Unlock()
}

// Stats lists the watched servers, in plan order, with what has been seen for each.
func (a *Agent) Stats() []ServerStats {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := make([]ServerStats, 0, len(a.order))
	for _, s := range a.order {
		out = append(out, ServerStats{Server: s, LastDetection: a.lastDetection[s.Name], LastWake: a.lastWake[s.Name]})
	}

	return out
}

func isOffline(state string) bool { return state == "offline" || state == "stopped" }

// Handle reacts to a join signal for the named server. Not safe for concurrent use; Run serialises calls.
func (a *Agent) Handle(ctx context.Context, name string) Outcome {
	log := a.opts.Log.With("server", name)

	a.mu.Lock()
	s, ok := a.servers[name]
	cooldown, maxWakes := a.cooldown, a.maxWakes
	a.mu.Unlock()

	if !ok {
		log.Error("trigger for a server that isn't configured")

		return Unknown
	}

	now := a.opts.Now()
	if now.Before(a.quietUntil[name]) {
		log.Debug("ignoring trigger", "until", a.quietUntil[name])

		return Quiet
	}

	state, err := a.panel.State(ctx, s.ID)
	if err != nil {
		a.quietUntil[name] = now.Add(errorBackoff)
		log.Error("could not read server state, not starting", "err", err)

		return Failed
	}

	if !isOffline(state) {
		backoff := notOfflineBackoff
		if state == "suspended" {
			backoff = suspendedBackoff
		}

		a.quietUntil[name] = now.Add(backoff)
		log.Debug("server is not offline, nothing to do", "state", state)

		return NotOffline
	}

	recent := a.wakes[name][:0]
	for _, t := range a.wakes[name] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	a.wakes[name] = recent

	if len(recent) >= maxWakes {
		a.quietUntil[name] = now.Add(rateLimitBackoff)
		log.Warn("wake limit reached, not starting", "limit", maxWakes, "per", "hour")

		return RateLimited
	}

	if a.opts.DryRun {
		a.wakes[name] = append(a.wakes[name], now)
		a.quietUntil[name] = now.Add(cooldown)
		log.Info("dry run: would start the server")

		return DryRun
	}

	if err := a.panel.Start(ctx, s.ID); err != nil {
		a.quietUntil[name] = now.Add(errorBackoff)
		log.Error("start request failed", "err", err)

		return Failed
	}

	a.wakes[name] = append(a.wakes[name], now)
	a.quietUntil[name] = now.Add(cooldown)
	log.Info("started the server", "wakes_this_hour", len(a.wakes[name]))

	a.mu.Lock()
	a.lastWake[name] = now
	a.mu.Unlock()

	if a.opts.OnWake != nil {
		go a.opts.OnWake(s, now)
	}

	return Started
}

// Run handles triggers one at a time until ctx is cancelled or the channel closes.
func (a *Agent) Run(ctx context.Context, triggers <-chan string) {
	for {
		select {
		case name, ok := <-triggers:
			if !ok {
				return
			}

			a.Handle(ctx, name)
		case <-ctx.Done():
			return
		}
	}
}

// Update is a new detector and set of servers for Watch to switch to.
type Update struct {
	Detector        *detector.Detector
	Servers         []Server
	Cooldown        time.Duration
	MaxWakesPerHour int
}

// Watch feeds packets through the current detector and hands triggers to the agent until ctx is cancelled or the
// packet channel closes. Updates (from the panel's config) swap the detector in; nothing is detected until the
// first one arrives. Panel calls run on their own goroutine so a slow panel never makes the capture drop packets.
func Watch(ctx context.Context, packets <-chan detector.Packet, updates <-chan Update, a *Agent) {
	triggers := make(chan string, 16)
	done := make(chan struct{})

	go func() {
		defer close(done)
		a.Run(ctx, triggers)
	}()

	defer func() {
		close(triggers)
		<-done
	}()

	var d *detector.Detector

	apply := func(u Update) {
		d = u.Detector
		a.SetPlan(u.Servers, u.Cooldown, u.MaxWakesPerHour)
	}

	for {
		select {
		case u := <-updates:
			apply(u)
		case p, ok := <-packets:
			if !ok {
				return
			}

			// A packet and a new plan can be ready at the same moment, and select picks at random. Take any
			// pending plan first, so a packet is never judged against a plan that has already been replaced.
			for drained := false; !drained; {
				select {
				case u := <-updates:
					apply(u)
				default:
					drained = true
				}
			}

			if d == nil {
				continue
			}

			name, hit := d.Observe(p)
			if !hit {
				continue
			}

			a.NoteDetection(name, p.At)
			// Debug, not Info: while players are connected this fires every holdoff, and Handle logs what it does.
			a.opts.Log.Debug("join attempt detected", "server", name, "from", p.Src.String(), "port", p.DstPort)

			select {
			case triggers <- name:
			default:
				a.opts.Log.Warn("trigger queue full, dropping", "server", name)
			}
		case <-ctx.Done():
			return
		}
	}
}
