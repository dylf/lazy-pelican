package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCaptureStatusStartsNotOK(t *testing.T) {
	ok, msg := NewCaptureStatus().Snapshot()

	if ok || msg == "" {
		t.Fatalf("ok=%v msg=%q", ok, msg)
	}
}

func TestCaptureStatusTransitions(t *testing.T) {
	st := NewCaptureStatus()

	st.Ready()

	if ok, msg := st.Snapshot(); !ok || msg != "" {
		t.Fatalf("after Ready: %v %q", ok, msg)
	}

	st.Fail(errors.New("no permission"))

	if ok, msg := st.Snapshot(); ok || msg != "no permission" {
		t.Fatalf("after Fail: %v %q", ok, msg)
	}
}

func TestSuperviseRetriesAfterFailureAndRecovers(t *testing.T) {
	st := NewCaptureStatus()

	var attempts atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	run := func(ctx context.Context, ready func()) error {
		if attempts.Add(1) < 3 {
			return errors.New("operation not permitted")
		}

		ready()
		<-ctx.Done()

		return nil
	}

	done := make(chan struct{})

	go func() {
		Supervise(ctx, run, st, 5*time.Millisecond, quiet())
		close(done)
	}()

	deadline := time.After(2 * time.Second)

	for {
		if ok, _ := st.Snapshot(); ok {
			break
		}

		select {
		case <-deadline:
			t.Fatalf("capture never recovered after %d attempts", attempts.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}

	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Supervise did not stop when the context was cancelled")
	}
}

func TestSuperviseReportsTheFailureReason(t *testing.T) {
	st := NewCaptureStatus()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Supervise(ctx, func(context.Context, func()) error { return errors.New("operation not permitted") }, st, time.Hour, quiet())

	deadline := time.After(2 * time.Second)

	for {
		if ok, msg := st.Snapshot(); !ok && msg == "operation not permitted" {
			return
		}

		select {
		case <-deadline:
			ok, msg := st.Snapshot()
			t.Fatalf("status never showed the failure: ok=%v msg=%q", ok, msg)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestSuperviseTreatsACleanReturnAsAFailure(t *testing.T) {
	st := NewCaptureStatus()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go Supervise(ctx, func(context.Context, func()) error { return nil }, st, time.Hour, quiet())

	time.Sleep(50 * time.Millisecond)

	if ok, msg := st.Snapshot(); ok || msg == "" {
		t.Fatalf("capture that stopped must not look healthy: ok=%v msg=%q", ok, msg)
	}
}
