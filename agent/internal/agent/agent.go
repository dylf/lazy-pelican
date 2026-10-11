// Package agent turns "someone is trying to join" signals into panel start requests, with safety limits.
package agent

import (
	"context"
	"log/slog"
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
	Servers         []Server
	Cooldown        time.Duration // after a start, ignore triggers for this long
	MaxWakesPerHour int
	DryRun          bool
	Now             func() time.Time
	Log             *slog.Logger
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
	opts       Options
	panel      Panel
	servers    map[string]Server
	quietUntil map[string]time.Time
	wakes      map[string][]time.Time
}

func New(p Panel, o Options) *Agent {
	if o.Now == nil {
		o.Now = time.Now
	}

	if o.Log == nil {
		o.Log = slog.Default()
	}

	a := &Agent{opts: o, panel: p, servers: map[string]Server{}, quietUntil: map[string]time.Time{}, wakes: map[string][]time.Time{}}
	for _, s := range o.Servers {
		a.servers[s.Name] = s
	}

	return a
}

func isOffline(state string) bool { return state == "offline" || state == "stopped" }

// Handle reacts to a join signal for the named server. Not safe for concurrent use; Run serialises calls.
func (a *Agent) Handle(ctx context.Context, name string) Outcome {
	log := a.opts.Log.With("server", name)

	s, ok := a.servers[name]
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

	if len(recent) >= a.opts.MaxWakesPerHour {
		a.quietUntil[name] = now.Add(rateLimitBackoff)
		log.Warn("wake limit reached, not starting", "limit", a.opts.MaxWakesPerHour, "per", "hour")

		return RateLimited
	}

	if a.opts.DryRun {
		a.wakes[name] = append(a.wakes[name], now)
		a.quietUntil[name] = now.Add(a.opts.Cooldown)
		log.Info("dry run: would start the server")

		return DryRun
	}

	if err := a.panel.Start(ctx, s.ID); err != nil {
		a.quietUntil[name] = now.Add(errorBackoff)
		log.Error("start request failed", "err", err)

		return Failed
	}

	a.wakes[name] = append(a.wakes[name], now)
	a.quietUntil[name] = now.Add(a.opts.Cooldown)
	log.Info("started the server", "wakes_this_hour", len(a.wakes[name]))

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

// Watch feeds packets through the detector and hands triggers to the agent until ctx is cancelled.
// Panel calls run on their own goroutine so a slow panel never makes the capture drop packets.
func Watch(ctx context.Context, d *detector.Detector, packets <-chan detector.Packet, a *Agent) {
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

	for {
		select {
		case p, ok := <-packets:
			if !ok {
				return
			}

			name, hit := d.Observe(p)
			if !hit {
				continue
			}

			a.opts.Log.Info("join attempt detected", "server", name, "from", p.Src.String(), "port", p.DstPort)

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
