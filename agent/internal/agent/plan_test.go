package agent

import (
	"net/netip"
	"testing"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/config"
	"github.com/dylf/lazy-pelican/agent/internal/panel"
)

func remote() *panel.RemoteConfig {
	r := &panel.RemoteConfig{APIVersion: 1}
	r.Detector.Packets, r.Detector.WindowSeconds, r.Detector.HoldoffSeconds = 4, 8, 20
	r.Limits.CooldownSeconds, r.Limits.MaxWakesPerHour = 120, 3
	r.Servers = []panel.RemoteServer{{Identifier: "abc123", Name: "Valheim", Node: "node", Ports: []uint16{27000}}}
	r.Servers[0].Match.Prefix, r.Servers[0].Match.Length = "2010000d", 512

	return r
}

func TestResolveDefaultsWithNoPanelAndNoServers(t *testing.T) {
	p := Resolve(&config.Config{}, nil)

	if p.Packets != 3 || p.Window != 5*time.Second || p.Holdoff != 30*time.Second || p.Cooldown != 5*time.Minute || p.MaxWakesPerHour != 6 || len(p.Servers) != 0 || p.FromPanel {
		t.Fatalf("%+v", p)
	}
}

func TestResolveUsesThePanel(t *testing.T) {
	p := Resolve(&config.Config{}, remote())

	if !p.FromPanel || p.Packets != 4 || p.Window != 8*time.Second || p.Holdoff != 20*time.Second || p.Cooldown != 2*time.Minute || p.MaxWakesPerHour != 3 {
		t.Fatalf("%+v", p)
	}

	if len(p.Servers) != 1 || p.Servers[0].ID != "abc123" || p.Servers[0].Name != "Valheim" || p.Servers[0].Match.Prefix != "2010000d" || p.Servers[0].Match.Length != 512 {
		t.Fatalf("servers %+v", p.Servers)
	}
}

func TestResolveAPanelHoldoffOfZeroIsRespected(t *testing.T) {
	r := remote()
	r.Detector.HoldoffSeconds = 0
	r.Limits.CooldownSeconds = 0

	if p := Resolve(&config.Config{}, r); p.Holdoff != 0 || p.Cooldown != 0 {
		t.Fatalf("zero must mean zero when the panel sent it: %+v", p)
	}
}

func TestLocalSettingsOverrideThePanel(t *testing.T) {
	local := &config.Config{}
	local.Detector.Packets, local.Detector.Window, local.Detector.Holdoff = 9, time.Second, time.Minute
	local.Limits.Cooldown, local.Limits.MaxWakesPerHour = time.Hour, 1

	p := Resolve(local, remote())

	if p.Packets != 9 || p.Window != time.Second || p.Holdoff != time.Minute || p.Cooldown != time.Hour || p.MaxWakesPerHour != 1 {
		t.Fatalf("%+v", p)
	}

	if !p.FromPanel || len(p.Servers) != 1 {
		t.Fatal("overriding thresholds must not change where the servers come from")
	}
}

func TestLocalServersOverrideThePanelsListCompletely(t *testing.T) {
	local := &config.Config{Servers: []config.Server{{Name: "mine", ID: "zzz", Ports: []uint16{1234}}}}

	p := Resolve(local, remote())

	if p.FromPanel || len(p.Servers) != 1 || p.Servers[0].ID != "zzz" {
		t.Fatalf("%+v", p)
	}
}

func TestServerWithoutANameFallsBackToItsIdentifier(t *testing.T) {
	r := remote()
	r.Servers[0].Name = ""

	if p := Resolve(&config.Config{}, r); p.Servers[0].Name != "abc123" {
		t.Fatalf("%+v", p.Servers[0])
	}
}

func TestCompile(t *testing.T) {
	p := Resolve(&config.Config{}, remote())

	d, servers, err := p.Compile([]netip.Prefix{netip.MustParsePrefix("10.0.0.50/32")})
	if err != nil {
		t.Fatal(err)
	}

	if len(servers) != 1 || servers[0] != (Server{Name: "Valheim", ID: "abc123"}) {
		t.Fatalf("servers %+v", servers)
	}

	if ports := d.Ports(); len(ports) != 1 || ports[0] != 27000 {
		t.Fatalf("ports %v", ports)
	}
}

func TestCompileRejectsBadInput(t *testing.T) {
	r := remote()
	r.Servers[0].Match.Prefix = "zz"

	if _, _, err := Resolve(&config.Config{}, r).Compile(nil); err == nil {
		t.Error("a non-hex prefix from the panel must be an error")
	}

	r = remote()
	r.Servers = append(r.Servers, panel.RemoteServer{Identifier: "def456", Name: "Other", Ports: []uint16{27000}})

	if _, _, err := Resolve(&config.Config{}, r).Compile(nil); err == nil {
		t.Error("two servers on one port must be an error")
	}
}
