package agent

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"reflect"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/config"
	"github.com/dylf/lazy-pelican/agent/internal/panel"
)

// RetryEvery is how soon to try again after a failure, instead of waiting for the full interval. At boot the
// panel is often unreachable for a few seconds while the network comes up, and the agent shouldn't sit idle for a
// whole refresh interval because of it. A variable so tests don't have to wait.
var RetryEvery = 10 * time.Second

func nextDelay(interval time.Duration, failing bool) time.Duration {
	if failing && RetryEvery < interval {
		return RetryEvery
	}

	return interval
}

// RunRefresh fetches the panel's config now and then every interval, and sends an Update whenever the resulting
// plan changes. If the panel can't be reached it keeps the last plan, so a panel restart never stops the agent
// from watching.
//
// apply is called with each new plan before its Update is sent (the caller uses it to retarget packet capture).
func RunRefresh(
	ctx context.Context,
	interval time.Duration,
	local *config.Config,
	fetch func(context.Context) (*panel.RemoteConfig, error),
	ignore []netip.Prefix,
	apply func(Plan),
	updates chan<- Update,
	log *slog.Logger,
) {
	var current *Plan

	failing := false

	tick := func() {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		remote, err := fetch(cctx)

		switch {
		case err != nil && !failing:
			failing = true

			if errors.Is(err, panel.ErrNotSupported) {
				log.Warn("the panel has no Lazy Pelican agent API; using the local config file only", "err", err)
			} else {
				log.Warn("could not fetch the config from the panel", "err", err, "using", "last known config")
			}
		case err == nil && failing:
			failing = false

			log.Info("fetching the config from the panel works again")
		}

		// With no answer and a plan already running, keep it. With no answer and nothing yet, use the local file.
		if err != nil && current != nil {
			return
		}

		if err != nil {
			remote = nil
		}

		plan := Resolve(local, remote)

		if current != nil && reflect.DeepEqual(*current, plan) {
			return
		}

		d, servers, cerr := plan.Compile(ignore)
		if cerr != nil {
			log.Error("the new config is invalid, keeping the old one", "err", cerr)

			return
		}

		current = &plan

		log.Info("watching", "servers", len(plan.Servers), "from_panel", plan.FromPanel,
			"packets", plan.Packets, "window", plan.Window, "holdoff", plan.Holdoff, "cooldown", plan.Cooldown, "max_wakes_per_hour", plan.MaxWakesPerHour)

		for _, s := range plan.Servers {
			log.Info("  server", "name", s.Name, "id", s.ID, "ports", s.Ports)
		}

		if apply != nil {
			apply(plan)
		}

		select {
		case updates <- Update{Detector: d, Servers: servers, Cooldown: plan.Cooldown, MaxWakesPerHour: plan.MaxWakesPerHour}:
		case <-ctx.Done():
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
