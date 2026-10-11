package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/panel"
)

func TestBuildHeartbeatWhenCaptureWorks(t *testing.T) {
	st := NewCaptureStatus()
	st.Ready()

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.FixedZone("x", 3600))
	hb := BuildHeartbeat("1.2.3", "gameserver", "node", "any", st, []ServerStats{
		{Server: Server{Name: "Valheim", ID: "abc123"}, LastDetection: at},
		{Server: Server{Name: "Palworld", ID: "def456"}},
	})

	if hb.AgentVersion != "1.2.3" || hb.Hostname != "gameserver" || hb.Node != "node" || hb.Interface != "any" || !hb.Capture.OK || hb.Capture.Error != nil {
		t.Fatalf("%+v", hb)
	}

	if len(hb.Servers) != 2 || hb.Servers[0].Identifier != "abc123" || hb.Servers[0].LastDetectionAt == nil || !hb.Servers[0].LastDetectionAt.Equal(at) || hb.Servers[0].LastDetectionAt.Location() != time.UTC {
		t.Fatalf("servers %+v", hb.Servers)
	}

	if hb.Servers[1].LastDetectionAt != nil || hb.Servers[1].LastWakeAt != nil {
		t.Fatal("a server that was never seen must report nulls, not the zero time")
	}
}

func TestBuildHeartbeatReportsACaptureFailure(t *testing.T) {
	st := NewCaptureStatus()
	st.Fail(errors.New("operation not permitted"))

	hb := BuildHeartbeat("1", "h", "", "any", st, nil)

	if hb.Capture.OK || hb.Capture.Error == nil || *hb.Capture.Error != "operation not permitted" {
		t.Fatalf("%+v", hb.Capture)
	}

	if hb.Servers == nil {
		t.Fatal("servers must be an empty list, not null")
	}
}

func TestRunHeartbeatsSendsImmediatelyThenRepeats(t *testing.T) {
	var sent atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go RunHeartbeats(ctx, 10*time.Millisecond, func() panel.Heartbeat { return panel.Heartbeat{} },
		func(context.Context, panel.Heartbeat) error { sent.Add(1); return nil }, quiet())

	deadline := time.After(2 * time.Second)

	for sent.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("only %d heartbeats sent", sent.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestRunHeartbeatsKeepsGoingAfterFailures(t *testing.T) {
	var sent atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go RunHeartbeats(ctx, 10*time.Millisecond, func() panel.Heartbeat { return panel.Heartbeat{} },
		func(context.Context, panel.Heartbeat) error { sent.Add(1); return panel.ErrNotSupported }, quiet())

	deadline := time.After(2 * time.Second)

	for sent.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("stopped retrying after a failure: %d sent", sent.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestRunHeartbeatsRetriesQuicklyAfterAFailure(t *testing.T) {
	old := RetryEvery
	RetryEvery = 10 * time.Millisecond

	t.Cleanup(func() { RetryEvery = old })

	var sent atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// An hour between heartbeats, but the first one fails: the second must come from the quick retry.
	go RunHeartbeats(ctx, time.Hour, func() panel.Heartbeat { return panel.Heartbeat{} },
		func(context.Context, panel.Heartbeat) error {
			if sent.Add(1) == 1 {
				return errors.New("connection refused")
			}

			return nil
		}, quiet())

	deadline := time.After(2 * time.Second)

	for sent.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("no quick retry after a failed heartbeat")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
