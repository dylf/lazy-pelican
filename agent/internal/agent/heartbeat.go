package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/panel"
)

// BuildHeartbeat assembles what the panel shows about this agent.
func BuildHeartbeat(version, hostname, node, iface string, st *CaptureStatus, stats []ServerStats) panel.Heartbeat {
	ok, msg := st.Snapshot()

	hb := panel.Heartbeat{
		AgentVersion: version,
		Hostname:     hostname,
		Node:         node,
		Interface:    iface,
		Capture:      panel.CaptureReport{OK: ok},
		Servers:      make([]panel.ServerReport, 0, len(stats)),
	}

	if !ok {
		hb.Capture.Error = &msg
	}

	for _, s := range stats {
		r := panel.ServerReport{Identifier: s.Server.ID}

		if !s.LastDetection.IsZero() {
			t := s.LastDetection.UTC()
			r.LastDetectionAt = &t
		}

		if !s.LastWake.IsZero() {
			t := s.LastWake.UTC()
			r.LastWakeAt = &t
		}

		hb.Servers = append(hb.Servers, r)
	}

	return hb
}

// RunHeartbeats sends a heartbeat now and then every interval until ctx is cancelled. Failures are logged when they
// start and when they clear, not on every attempt.
func RunHeartbeats(ctx context.Context, interval time.Duration, build func() panel.Heartbeat, send func(context.Context, panel.Heartbeat) error, log *slog.Logger) {
	failing := false

	tick := func() {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		err := send(cctx, build())

		switch {
		case err != nil && !failing:
			failing = true

			if errors.Is(err, panel.ErrNotSupported) {
				log.Warn("the panel can't receive heartbeats; update or enable the Lazy Pelican plugin to see this agent's status there", "err", err)
			} else {
				log.Warn("could not send a heartbeat to the panel", "err", err)
			}
		case err == nil && failing:
			failing = false

			log.Info("heartbeats to the panel are working again")
		}
	}

	for {
		tick()

		select {
		case <-time.After(nextDelay(interval, failing)):
		case <-ctx.Done():
			return
		}
	}
}
