package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/agent"
	"github.com/dylf/lazy-pelican/agent/internal/config"
	"github.com/dylf/lazy-pelican/agent/internal/panel"
	"github.com/dylf/lazy-pelican/agent/internal/sniff"
)

// doctor checks everything run depends on, without starting any server. Most setup mistakes would otherwise be
// silent: the agent would run happily and simply never wake anything.
func doctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	path := fs.String("config", defaultConfig, "config file")
	fs.Parse(args)

	failed := false
	check := func(ok bool, msg string, args ...any) {
		mark := "ok  "
		if !ok {
			mark, failed = "FAIL", true
		}

		fmt.Printf("[%s] %s\n", mark, fmt.Sprintf(msg, args...))
	}
	warn := func(msg string, args ...any) { fmt.Printf("[warn] %s\n", fmt.Sprintf(msg, args...)) }

	cfg, err := config.Load(*path)
	if err != nil {
		check(false, "config %s: %v", *path, err)

		return 1
	}

	check(true, "config loaded: %s", cfg.Redacted())

	if err := sniff.Check(cfg.Interface); err != nil {
		check(false, "packet capture on %q: %v", cfg.Interface, err)
	} else {
		check(true, "packet capture on %q works", cfg.Interface)
	}

	ips := panelAddresses(cfg.PanelURL)
	if len(ips) == 0 {
		check(false, "could not resolve the panel's address, so its own traffic can't be ignored (add it to ignore_sources)")
	} else {
		check(true, "panel resolves to %v and is ignored as a source", ips)

		configured, _ := cfg.IgnorePrefixes()
		public := true

		for _, p := range append(append([]netip.Prefix{}, ips...), configured...) {
			if p.Addr().IsPrivate() || p.Addr().IsLoopback() {
				public = false
			}
		}

		if public {
			warn("the panel resolves to a public address, but its own traffic reaches this host from its LAN address. " +
				"Add that address to ignore_sources so the panel's queries are never counted.")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := panel.New(cfg.PanelURL, cfg.APIKey)

	if err := client.Ping(ctx); err != nil {
		check(false, "panel API key: %v", err)

		return 1
	}

	check(true, "panel accepts the API key")

	remote, err := client.Config(ctx, cfg.Node)

	switch {
	case errors.Is(err, panel.ErrNotSupported):
		if len(cfg.Servers) == 0 {
			check(false, "%v. Install or update the Lazy Pelican plugin, or list servers in this config file.", err)
		} else {
			warn("%v. Using the servers in this file; the panel won't show this agent's status.", err)
		}
	case err != nil:
		check(false, "fetching the config from the panel: %v", err)
	default:
		check(true, "fetched the config from the panel (%d server(s) with wake on connect for this key's user)", len(remote.Servers))
	}

	plan := agent.Resolve(cfg, remote)

	if plan.FromPanel {
		nodes := map[string]bool{}
		for _, s := range remote.Servers {
			nodes[s.Node] = true
		}

		if cfg.Node == "" && len(nodes) > 1 {
			check(false, "the panel lists servers on %d nodes but `node:` is not set, so this agent would watch ports for servers on other hosts. Set node: to this host's node.", len(nodes))
		}
	} else if len(cfg.Servers) > 0 {
		warn("servers are listed in this file, so the panel's list is ignored (local override).")
	}

	if len(plan.Servers) == 0 {
		check(false, "there are no servers to watch. Turn on \"Wake on connect\" for a server on its Lazy Pelican page, and make sure this key's user has the Start permission on it.")
	}

	for _, s := range plan.Servers {
		d, err := client.Details(ctx, s.ID)
		if err != nil {
			check(false, "server %q (%s): %v", s.Name, s.ID, err)

			continue
		}

		check(true, "server %q is %q on the panel, watching port(s) %v", s.Name, d.Name, s.Ports)
		check(d.CanStart, "server %q: this API key's user may start it (needs the control.start permission)", s.Name)

		state, err := client.State(ctx, s.ID)
		check(err == nil, "server %q: current state %q %v", s.Name, state, errOrEmpty(err))
	}

	if failed {
		fmt.Println("\nSome checks failed.")

		return 1
	}

	fmt.Println("\nAll checks passed.")

	return 0
}

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}

	return "(" + err.Error() + ")"
}
