package agent

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// CaptureStatus is whether packet capture is working right now, so heartbeats can tell the panel.
type CaptureStatus struct {
	mu  sync.Mutex
	ok  bool
	err string
}

func NewCaptureStatus() *CaptureStatus { return &CaptureStatus{err: "capture has not started yet"} }

func (c *CaptureStatus) Ready() {
	c.mu.Lock()
	c.ok, c.err = true, ""
	c.mu.Unlock()
}

func (c *CaptureStatus) Fail(err error) {
	c.mu.Lock()
	c.ok, c.err = false, err.Error()
	c.mu.Unlock()
}

func (c *CaptureStatus) Snapshot() (ok bool, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ok, c.err
}

// Supervise keeps packet capture running. If it fails (no permission, interface gone) it records why and tries
// again after retry, instead of exiting: the agent stays alive and tells the panel what's wrong, so the problem
// is visible there rather than only in a log on the host.
func Supervise(ctx context.Context, run func(ctx context.Context, ready func()) error, st *CaptureStatus, retry time.Duration, log *slog.Logger) {
	for ctx.Err() == nil {
		err := run(ctx, st.Ready)
		if ctx.Err() != nil {
			return
		}

		if err == nil {
			err = errors.New("capture stopped unexpectedly")
		}

		st.Fail(err)
		log.Error("packet capture is not working, will retry", "err", err, "retry_in", retry)

		select {
		case <-time.After(retry):
		case <-ctx.Done():
			return
		}
	}
}
