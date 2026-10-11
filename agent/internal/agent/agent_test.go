package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/detector"
)

type fakePanel struct {
	mu       sync.Mutex
	state    string
	stateErr error
	startErr error
	states   int
	starts   int
}

func (f *fakePanel) State(context.Context, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states++

	return f.state, f.stateErr
}

func (f *fakePanel) Start(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++

	return f.startErr
}

func (f *fakePanel) counts() (states, starts int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.states, f.starts
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func setup(p *fakePanel, mods ...func(*Options)) (*Agent, *clock) {
	c := &clock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	o := Options{
		Servers:         []Server{{Name: "valheim", ID: "abc123"}},
		Cooldown:        5 * time.Minute,
		MaxWakesPerHour: 3,
		Now:             c.now,
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	for _, m := range mods {
		m(&o)
	}

	return New(p, o), c
}

func TestStartsAnOfflineServer(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, _ := setup(p)

	if got := a.Handle(context.Background(), "valheim"); got != Started {
		t.Fatalf("got %s", got)
	}

	if _, starts := p.counts(); starts != 1 {
		t.Fatalf("expected 1 start, got %d", starts)
	}
}

func TestNeverStartsAServerThatIsNotOffline(t *testing.T) {
	for _, state := range []string{"running", "starting", "stopping", "suspended"} {
		p := &fakePanel{state: state}
		a, _ := setup(p)

		if got := a.Handle(context.Background(), "valheim"); got != NotOffline {
			t.Errorf("%s: got %s", state, got)
		}

		if _, starts := p.counts(); starts != 0 {
			t.Errorf("%s: a start was sent", state)
		}
	}
}

func TestCooldownAfterStartSuppressesFurtherTriggers(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, c := setup(p)

	a.Handle(context.Background(), "valheim")
	c.advance(time.Minute)

	// The panel's state is cached up to 20s, so it may still say "offline" right after a start.
	if got := a.Handle(context.Background(), "valheim"); got != Quiet {
		t.Fatalf("got %s, want quiet", got)
	}

	if _, starts := p.counts(); starts != 1 {
		t.Fatalf("expected exactly 1 start, got %d", starts)
	}

	c.advance(5 * time.Minute)
	if got := a.Handle(context.Background(), "valheim"); got != Started {
		t.Fatalf("after the cooldown got %s, want started", got)
	}
}

func TestHourlyCap(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, c := setup(p, func(o *Options) { o.Cooldown = time.Minute })

	for i := 0; i < 3; i++ {
		if got := a.Handle(context.Background(), "valheim"); got != Started {
			t.Fatalf("wake %d: got %s", i+1, got)
		}

		c.advance(2 * time.Minute)
	}

	if got := a.Handle(context.Background(), "valheim"); got != RateLimited {
		t.Fatalf("4th wake inside an hour: got %s, want rate-limited", got)
	}

	if _, starts := p.counts(); starts != 3 {
		t.Fatalf("expected 3 starts, got %d", starts)
	}

	c.advance(time.Hour)
	if got := a.Handle(context.Background(), "valheim"); got != Started {
		t.Fatalf("after the hour got %s, want started", got)
	}
}

func TestStateErrorNeverStartsAndBacksOff(t *testing.T) {
	p := &fakePanel{stateErr: errors.New("panel down")}
	a, c := setup(p)

	if got := a.Handle(context.Background(), "valheim"); got != Failed {
		t.Fatalf("got %s", got)
	}

	if got := a.Handle(context.Background(), "valheim"); got != Quiet {
		t.Fatalf("immediately after an error got %s, want quiet", got)
	}

	if states, starts := p.counts(); states != 1 || starts != 0 {
		t.Fatalf("states=%d starts=%d", states, starts)
	}

	c.advance(time.Minute)
	a.Handle(context.Background(), "valheim")

	if states, _ := p.counts(); states != 2 {
		t.Fatalf("should retry after the back-off, states=%d", states)
	}
}

func TestFailedStartIsRetriedSoonAndNotCountedAgainstTheCap(t *testing.T) {
	p := &fakePanel{state: "offline", startErr: errors.New("403")}
	a, c := setup(p)

	if got := a.Handle(context.Background(), "valheim"); got != Failed {
		t.Fatalf("got %s", got)
	}

	p.mu.Lock()
	p.startErr = nil
	p.mu.Unlock()
	c.advance(time.Minute)

	if got := a.Handle(context.Background(), "valheim"); got != Started {
		t.Fatalf("retry got %s", got)
	}
}

func TestDryRunNeverStarts(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, _ := setup(p, func(o *Options) { o.DryRun = true })

	if got := a.Handle(context.Background(), "valheim"); got != DryRun {
		t.Fatalf("got %s", got)
	}

	if _, starts := p.counts(); starts != 0 {
		t.Fatal("dry run sent a start")
	}
}

func TestRunningServerIsNotPolledForEveryTrigger(t *testing.T) {
	p := &fakePanel{state: "running"}
	a, c := setup(p)

	for i := 0; i < 20; i++ { // players connected: triggers keep arriving
		a.Handle(context.Background(), "valheim")
		c.advance(time.Second)
	}

	if states, _ := p.counts(); states > 2 {
		t.Fatalf("asked the panel %d times in 20s, expected the back-off to limit it", states)
	}
}

func TestUnknownServer(t *testing.T) {
	a, _ := setup(&fakePanel{})

	if got := a.Handle(context.Background(), "nope"); got != Unknown {
		t.Fatalf("got %s", got)
	}
}

func joinBurst(src string, port uint16, n int) chan detector.Packet {
	packets := make(chan detector.Packet, n)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < n; i++ {
		packets <- detector.Packet{At: start.Add(time.Duration(i) * time.Second), Src: netip.MustParseAddr(src), DstPort: port, Length: 1, Payload: []byte{1}}
	}

	close(packets)

	return packets
}

func oneServerUpdate(t *testing.T) Update {
	t.Helper()

	d, err := detector.New([]detector.Target{{Name: "valheim", Ports: []uint16{27000}, Rule: detector.Rule{Packets: 3, Window: 5 * time.Second, Holdoff: 30 * time.Second}}})
	if err != nil {
		t.Fatal(err)
	}

	return Update{Detector: d, Servers: []Server{{Name: "valheim", ID: "abc123"}}, Cooldown: 5 * time.Minute, MaxWakesPerHour: 3}
}

func TestWatchEndToEnd(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, _ := setup(p)

	updates := make(chan Update, 1)
	updates <- oneServerUpdate(t)

	Watch(context.Background(), joinBurst("198.51.100.9", 27000, 3), updates, a) // returns once the channel is drained

	if _, starts := p.counts(); starts != 1 {
		t.Fatalf("expected the join attempt to start the server once, got %d", starts)
	}
}

func TestWatchIgnoresPacketsUntilItHasAPlan(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, _ := setup(p)

	Watch(context.Background(), joinBurst("198.51.100.9", 27000, 10), make(chan Update), a)

	if states, starts := p.counts(); states != 0 || starts != 0 {
		t.Fatalf("acted before it had a plan: states=%d starts=%d", states, starts)
	}
}

func TestWatchSwitchesToANewPlan(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, _ := setup(p, func(o *Options) { o.Servers = nil })

	updates := make(chan Update, 2)
	updates <- oneServerUpdate(t)

	// a second plan that no longer watches the server: traffic must stop mattering
	empty, _ := detector.New(nil)
	updates <- Update{Detector: empty}

	Watch(context.Background(), joinBurst("198.51.100.9", 27000, 5), updates, a)

	if _, starts := p.counts(); starts != 0 {
		t.Fatalf("a removed server was still woken: %d starts", starts)
	}
}

func TestSetPlanKeepsRunningCooldowns(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, c := setup(p)

	a.Handle(context.Background(), "valheim") // starts it, cooldown begins

	a.SetPlan([]Server{{Name: "valheim", ID: "abc123"}}, 5*time.Minute, 3) // e.g. a config refresh
	c.advance(time.Minute)

	if got := a.Handle(context.Background(), "valheim"); got != Quiet {
		t.Fatalf("a config refresh reset the cooldown: got %s", got)
	}
}

func TestSetPlanAppliesNewLimits(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, c := setup(p)

	a.SetPlan([]Server{{Name: "valheim", ID: "abc123"}}, time.Minute, 1)

	a.Handle(context.Background(), "valheim")
	c.advance(2 * time.Minute)

	if got := a.Handle(context.Background(), "valheim"); got != RateLimited {
		t.Fatalf("got %s, want rate-limited under the new cap of 1", got)
	}
}

func TestOnWakeIsCalledOnlyForRealStarts(t *testing.T) {
	woke := make(chan Server, 4)

	p := &fakePanel{state: "offline"}
	a, _ := setup(p, func(o *Options) { o.OnWake = func(s Server, _ time.Time) { woke <- s } })

	a.Handle(context.Background(), "valheim")

	select {
	case s := <-woke:
		if s.ID != "abc123" {
			t.Fatalf("woke %+v", s)
		}
	case <-time.After(time.Second):
		t.Fatal("OnWake was not called after a start")
	}

	for name, mod := range map[string]func(*Options){"dry run": func(o *Options) { o.DryRun = true }} {
		p2 := &fakePanel{state: "offline"}
		b, _ := setup(p2, mod, func(o *Options) { o.OnWake = func(s Server, _ time.Time) { woke <- s } })
		b.Handle(context.Background(), "valheim")

		select {
		case <-woke:
			t.Fatalf("%s: OnWake was called without a start", name)
		case <-time.After(100 * time.Millisecond):
		}
	}

	p3 := &fakePanel{state: "running"}
	c, _ := setup(p3, func(o *Options) { o.OnWake = func(s Server, _ time.Time) { woke <- s } })
	c.Handle(context.Background(), "valheim")

	select {
	case <-woke:
		t.Fatal("OnWake was called for a server that was already running")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStatsTracksDetectionsAndWakes(t *testing.T) {
	p := &fakePanel{state: "offline"}
	a, c := setup(p)

	if s := a.Stats(); len(s) != 1 || !s[0].LastDetection.IsZero() || !s[0].LastWake.IsZero() {
		t.Fatalf("fresh stats %+v", s)
	}

	at := c.now().Add(-time.Minute)
	a.NoteDetection("valheim", at)
	a.Handle(context.Background(), "valheim")

	s := a.Stats()
	if !s[0].LastDetection.Equal(at) || !s[0].LastWake.Equal(c.now()) {
		t.Fatalf("stats %+v", s)
	}
}
