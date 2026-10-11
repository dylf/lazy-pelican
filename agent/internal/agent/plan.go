package agent

import (
	"encoding/hex"
	"net/netip"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/config"
	"github.com/dylf/lazy-pelican/agent/internal/detector"
	"github.com/dylf/lazy-pelican/agent/internal/panel"
)

// Defaults used when neither the panel nor the local file say otherwise.
const (
	defaultPackets  = 3
	defaultWindow   = 5 * time.Second
	defaultHoldoff  = 30 * time.Second
	defaultCooldown = 5 * time.Minute
	defaultMaxWakes = 6
)

// Plan is everything the agent needs to run: what to watch, how to detect, and the safety limits.
type Plan struct {
	Servers         []config.Server
	Packets         int
	Window          time.Duration
	Holdoff         time.Duration
	Cooldown        time.Duration
	MaxWakesPerHour int
	FromPanel       bool // servers came from the panel (false: the local file overrides them)
}

// Resolve combines the local config with what the panel said. remote may be nil (panel unreachable or too old).
//
// Precedence for each setting: a non-zero value in the local file, then the panel's value, then the built-in default.
// If the local file lists servers, it overrides the panel's server list completely.
func Resolve(local *config.Config, remote *panel.RemoteConfig) Plan {
	p := Plan{Packets: defaultPackets, Window: defaultWindow, Holdoff: defaultHoldoff, Cooldown: defaultCooldown, MaxWakesPerHour: defaultMaxWakes}

	if remote != nil {
		if remote.Detector.Packets > 0 {
			p.Packets = remote.Detector.Packets
		}

		if remote.Detector.WindowSeconds > 0 {
			p.Window = time.Duration(remote.Detector.WindowSeconds) * time.Second
		}

		if remote.Detector.HoldoffSeconds >= 0 && remote.APIVersion > 0 {
			p.Holdoff = time.Duration(remote.Detector.HoldoffSeconds) * time.Second
		}

		if remote.Limits.CooldownSeconds >= 0 && remote.APIVersion > 0 {
			p.Cooldown = time.Duration(remote.Limits.CooldownSeconds) * time.Second
		}

		if remote.Limits.MaxWakesPerHour > 0 {
			p.MaxWakesPerHour = remote.Limits.MaxWakesPerHour
		}
	}

	if local.Detector.Packets > 0 {
		p.Packets = local.Detector.Packets
	}

	if local.Detector.Window > 0 {
		p.Window = local.Detector.Window
	}

	if local.Detector.Holdoff > 0 {
		p.Holdoff = local.Detector.Holdoff
	}

	if local.Limits.Cooldown > 0 {
		p.Cooldown = local.Limits.Cooldown
	}

	if local.Limits.MaxWakesPerHour > 0 {
		p.MaxWakesPerHour = local.Limits.MaxWakesPerHour
	}

	switch {
	case len(local.Servers) > 0:
		p.Servers = local.Servers
	case remote != nil:
		p.FromPanel = true

		for _, s := range remote.Servers {
			name := s.Name
			if name == "" {
				name = s.Identifier
			}

			srv := config.Server{Name: name, ID: s.Identifier, Ports: s.Ports}
			srv.Match.Prefix, srv.Match.Length = s.Match.Prefix, s.Match.Length
			p.Servers = append(p.Servers, srv)
		}
	}

	return p
}

// Compile turns a plan into a detector, and the list of servers the agent can start.
// ignore are sources that never count (the panel's own address, plus ignore_sources).
func (p Plan) Compile(ignore []netip.Prefix) (*detector.Detector, []Server, error) {
	targets := make([]detector.Target, 0, len(p.Servers))
	servers := make([]Server, 0, len(p.Servers))

	for _, s := range p.Servers {
		prefix, err := hex.DecodeString(s.Match.Prefix)
		if err != nil {
			return nil, nil, err
		}

		targets = append(targets, detector.Target{
			Name:   s.Name,
			Ports:  s.Ports,
			Ignore: ignore,
			Rule:   detector.Rule{Packets: p.Packets, Window: p.Window, Holdoff: p.Holdoff, Prefix: prefix, Length: s.Match.Length},
		})
		servers = append(servers, Server{Name: s.Name, ID: s.ID})
	}

	d, err := detector.New(targets)
	if err != nil {
		return nil, nil, err
	}

	return d, servers, nil
}
