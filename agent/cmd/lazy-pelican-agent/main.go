// lazy-pelican-agent watches a Pelican node for players trying to join a stopped game server and asks the panel to start it.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/agent"
	"github.com/dylf/lazy-pelican/agent/internal/config"
	"github.com/dylf/lazy-pelican/agent/internal/detector"
	"github.com/dylf/lazy-pelican/agent/internal/panel"
	"github.com/dylf/lazy-pelican/agent/internal/sniff"
)

const (
	refreshEvery   = time.Minute
	heartbeatEvery = 30 * time.Second
	captureRetry   = 30 * time.Second
)

var version = "dev"

const defaultConfig = "/etc/lazy-pelican-agent/config.yml"

func usage() {
	fmt.Fprintf(os.Stderr, `lazy-pelican-agent %s

Usage:
  lazy-pelican-agent run     [-config FILE] [-dry-run] [-v]   watch for join attempts and start servers
  lazy-pelican-agent doctor  [-config FILE]                   check the setup without starting anything
  lazy-pelican-agent version
`, version)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "run":
		os.Exit(run(os.Args[2:]))
	case "doctor":
		os.Exit(doctor(os.Args[2:]))
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

// panelAddresses resolves the panel's host so its own traffic (e.g. Player Counter queries) never counts.
func panelAddresses(panelURL string) []netip.Prefix {
	u, err := url.Parse(panelURL)
	if err != nil {
		return nil
	}

	var out []netip.Prefix

	ips, _ := net.LookupIP(u.Hostname())
	for _, ip := range ips {
		if a, ok := netip.AddrFromSlice(ip); ok {
			a = a.Unmap()
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}

	return out
}

// ignoredSources is everything that must never count as a player: ignore_sources plus the panel's own addresses.
func ignoredSources(c *config.Config) ([]netip.Prefix, error) {
	ignore, err := c.IgnorePrefixes()
	if err != nil {
		return nil, err
	}

	return append(ignore, panelAddresses(c.PanelURL)...), nil
}

func allPorts(p agent.Plan) []uint16 {
	var ports []uint16

	for _, s := range p.Servers {
		ports = append(ports, s.Ports...)
	}

	return ports
}

func run(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	path := fs.String("config", defaultConfig, "config file")
	dry := fs.Bool("dry-run", false, "log what would be started, but never start anything")
	verbose := fs.Bool("v", false, "debug logging")
	fs.Parse(args)

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	cfg, err := config.Load(*path)
	if err != nil {
		log.Error("config", "err", err)

		return 1
	}

	ignore, err := ignoredSources(cfg)
	if err != nil {
		log.Error("config", "err", err)

		return 1
	}

	hostname, _ := os.Hostname()
	client := panel.New(cfg.PanelURL, cfg.APIKey)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := agent.New(client, agent.Options{
		DryRun: *dry,
		Log:    log,
		OnWake: func(s agent.Server, at time.Time) {
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			if err := client.ReportWake(cctx, s.ID, at); err != nil {
				log.Warn("could not record the wake in the panel's activity log", "server", s.Name, "err", err)
			}
		},
	})

	log.Info("starting", "version", version, "config", cfg.Redacted(), "dry_run", *dry)

	ports := sniff.NewPortSet()
	packets := make(chan detector.Packet, 256)
	updates := make(chan agent.Update, 1)
	status := agent.NewCaptureStatus()

	go agent.Supervise(ctx, func(ctx context.Context, ready func()) error {
		return sniff.Run(ctx, cfg.Interface, ports, packets, ready)
	}, status, captureRetry, log)

	go agent.RunRefresh(ctx, refreshEvery, cfg, func(ctx context.Context) (*panel.RemoteConfig, error) {
		return client.Config(ctx, cfg.Node)
	}, ignore, func(p agent.Plan) { ports.Set(allPorts(p)) }, updates, log)

	go agent.RunHeartbeats(ctx, heartbeatEvery, func() panel.Heartbeat {
		return agent.BuildHeartbeat(version, hostname, cfg.Node, cfg.Interface, status, a.Stats())
	}, client.SendHeartbeat, log)

	agent.Watch(ctx, packets, updates, a)

	return 0
}
