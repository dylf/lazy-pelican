package agent

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/config"
	"github.com/dylf/lazy-pelican/agent/internal/panel"
)

type fetcher struct {
	mu   sync.Mutex
	cfg  *panel.RemoteConfig
	err  error
	hits int
}

func (f *fetcher) fetch(context.Context) (*panel.RemoteConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits++

	return f.cfg, f.err
}

func (f *fetcher) set(cfg *panel.RemoteConfig, err error) {
	f.mu.Lock()
	f.cfg, f.err = cfg, err
	f.mu.Unlock()
}

func startRefresh(t *testing.T, local *config.Config, f *fetcher) (<-chan Update, *[]Plan, *sync.Mutex) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	updates := make(chan Update, 8)

	var (
		mu      sync.Mutex
		applied []Plan
	)

	go RunRefresh(ctx, 10*time.Millisecond, local, f.fetch, []netip.Prefix{netip.MustParsePrefix("10.0.0.50/32")},
		func(p Plan) { mu.Lock(); applied = append(applied, p); mu.Unlock() }, updates, quiet())

	return updates, &applied, &mu
}

func next(t *testing.T, ch <-chan Update) Update {
	t.Helper()

	select {
	case u := <-ch:
		return u
	case <-time.After(2 * time.Second):
		t.Fatal("no update arrived")

		return Update{}
	}
}

func TestRefreshSendsTheFirstPlanImmediately(t *testing.T) {
	f := &fetcher{cfg: remote()}
	updates, applied, mu := startRefresh(t, &config.Config{}, f)

	u := next(t, updates)

	if len(u.Servers) != 1 || u.Servers[0].ID != "abc123" || u.Cooldown != 2*time.Minute || u.MaxWakesPerHour != 3 {
		t.Fatalf("%+v", u)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(*applied) != 1 || len((*applied)[0].Servers) != 1 {
		t.Fatalf("apply not called with the plan: %+v", *applied)
	}
}

func TestRefreshOnlySendsWhenThePlanChanges(t *testing.T) {
	f := &fetcher{cfg: remote()}
	updates, _, _ := startRefresh(t, &config.Config{}, f)

	next(t, updates)

	select {
	case <-updates:
		t.Fatal("an unchanged config produced another update")
	case <-time.After(100 * time.Millisecond):
	}

	changed := remote()
	changed.Servers = append(changed.Servers, panel.RemoteServer{Identifier: "def456", Name: "Palworld", Ports: []uint16{27002}})
	f.set(changed, nil)

	if u := next(t, updates); len(u.Servers) != 2 {
		t.Fatalf("the change was not picked up: %+v", u.Servers)
	}
}

func TestRefreshKeepsTheLastPlanWhenThePanelGoesAway(t *testing.T) {
	f := &fetcher{cfg: remote()}
	updates, _, _ := startRefresh(t, &config.Config{}, f)

	next(t, updates)
	f.set(nil, errors.New("connection refused"))

	select {
	case u := <-updates:
		t.Fatalf("a panel outage must not change what is being watched: %+v", u)
	case <-time.After(150 * time.Millisecond):
	}

	f.set(remote(), nil)

	select {
	case u := <-updates:
		t.Fatalf("recovery with the same config must not produce an update: %+v", u)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRefreshFallsBackToTheLocalFileWhenThePanelIsUnreachableAtStart(t *testing.T) {
	local := &config.Config{Servers: []config.Server{{Name: "mine", ID: "zzz", Ports: []uint16{1234}}}}
	f := &fetcher{err: errors.New("connection refused")}
	updates, _, _ := startRefresh(t, local, f)

	if u := next(t, updates); len(u.Servers) != 1 || u.Servers[0].ID != "zzz" {
		t.Fatalf("%+v", u)
	}
}

func TestRefreshWithAnOldPluginUsesTheLocalFile(t *testing.T) {
	local := &config.Config{Servers: []config.Server{{Name: "mine", ID: "zzz", Ports: []uint16{1234}}}}
	f := &fetcher{err: panel.ErrNotSupported}
	updates, _, _ := startRefresh(t, local, f)

	if u := next(t, updates); len(u.Servers) != 1 || u.Servers[0].ID != "zzz" {
		t.Fatalf("%+v", u)
	}
}

func TestRefreshRejectsAnInvalidNewConfigAndKeepsTheOldOne(t *testing.T) {
	f := &fetcher{cfg: remote()}
	updates, _, _ := startRefresh(t, &config.Config{}, f)

	first := next(t, updates)

	bad := remote()
	bad.Servers[0].Match.Prefix = "not-hex"
	f.set(bad, nil)

	select {
	case u := <-updates:
		t.Fatalf("an invalid config replaced the working one: %+v", u)
	case <-time.After(150 * time.Millisecond):
	}

	if first.Detector == nil {
		t.Fatal("first update had no detector")
	}
}

func TestRefreshRetriesQuicklyAfterAFailureAtStartup(t *testing.T) {
	old := RetryEvery
	RetryEvery = 10 * time.Millisecond

	t.Cleanup(func() { RetryEvery = old })

	// The normal interval is an hour: only the fast retry can make this test pass.
	f := &fetcher{err: errors.New("connection refused")}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	updates := make(chan Update, 4)

	go RunRefresh(ctx, time.Hour, &config.Config{}, f.fetch, nil, nil, updates, quiet())

	// the first plan (empty, from the local file) arrives straight away
	next(t, updates)

	f.set(remote(), nil)

	if u := next(t, updates); len(u.Servers) != 1 {
		t.Fatalf("the panel's servers were not picked up on the quick retry: %+v", u)
	}
}

func TestNextDelay(t *testing.T) {
	old := RetryEvery
	RetryEvery = 10 * time.Second

	t.Cleanup(func() { RetryEvery = old })

	if nextDelay(time.Minute, false) != time.Minute {
		t.Error("healthy: use the normal interval")
	}

	if nextDelay(time.Minute, true) != 10*time.Second {
		t.Error("failing: retry quickly")
	}

	if nextDelay(5*time.Second, true) != 5*time.Second {
		t.Error("a retry delay longer than the interval must not slow things down")
	}
}
