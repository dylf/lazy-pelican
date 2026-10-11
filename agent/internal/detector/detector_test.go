package detector

import (
	"net/netip"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func addr(s string) netip.Addr { return netip.MustParseAddr(s) }

func pkt(at time.Duration, src string, port uint16, payload []byte) Packet {
	return Packet{At: t0.Add(at), Src: addr(src), DstPort: port, Length: len(payload), Payload: payload}
}

func target(mods ...func(*Target)) Target {
	t := Target{Name: "game", Ports: []uint16{27000}, Rule: Rule{Packets: 3, Window: 5 * time.Second, Holdoff: 30 * time.Second}}
	for _, m := range mods {
		m(&t)
	}

	return t
}

func mustNew(t *testing.T, targets ...Target) *Detector {
	t.Helper()

	d, err := New(targets)
	if err != nil {
		t.Fatal(err)
	}

	return d
}

func fires(d *Detector, packets ...Packet) (n int) {
	for _, p := range packets {
		if _, ok := d.Observe(p); ok {
			n++
		}
	}

	return n
}

func TestTriggersOnThirdPacketFromOneSource(t *testing.T) {
	d := mustNew(t, target())

	if _, ok := d.Observe(pkt(0, "198.51.100.1", 27000, []byte{1})); ok {
		t.Fatal("fired on first packet")
	}

	if _, ok := d.Observe(pkt(time.Second, "198.51.100.1", 27000, []byte{2})); ok {
		t.Fatal("fired on second packet")
	}

	name, ok := d.Observe(pkt(2*time.Second, "198.51.100.1", 27000, []byte{3}))
	if !ok || name != "game" {
		t.Fatalf("expected a trigger for game, got %q %v", name, ok)
	}
}

func TestSinglePacketScannerNeverTriggers(t *testing.T) {
	d := mustNew(t, target())

	if fires(d, pkt(0, "198.51.100.1", 27000, []byte{1})) != 0 {
		t.Fatal("a single probe must not trigger")
	}
}

func TestPacketsMustBeInsideTheWindow(t *testing.T) {
	d := mustNew(t, target())

	// one packet every 3s: never 3 inside a 5s window
	var packets []Packet
	for i := 0; i < 10; i++ {
		packets = append(packets, pkt(time.Duration(i)*3*time.Second, "198.51.100.1", 27000, []byte{1}))
	}

	if fires(d, packets...) != 0 {
		t.Fatal("slow trickle must not trigger")
	}
}

func TestSourcesAreCountedSeparately(t *testing.T) {
	d := mustNew(t, target())

	// three different sources, two packets each: no single source reaches 3
	var packets []Packet
	for i, src := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"} {
		packets = append(packets, pkt(time.Duration(i)*100*time.Millisecond, src, 27000, []byte{1}), pkt(time.Duration(i)*100*time.Millisecond+time.Second, src, 27000, []byte{1}))
	}

	if fires(d, packets...) != 0 {
		t.Fatal("packets from different sources must not add up")
	}
}

func TestQueriesAreIgnored(t *testing.T) {
	d := mustNew(t, target())
	query := []byte{0xff, 0xff, 0xff, 0xff, 'T', 'S', 'o', 'u', 'r', 'c', 'e'}

	var packets []Packet
	for i := 0; i < 50; i++ {
		packets = append(packets, pkt(time.Duration(i)*50*time.Millisecond, "198.51.100.1", 27000, query))
	}

	if fires(d, packets...) != 0 {
		t.Fatal("an A2S query flood must not wake the server")
	}
}

func TestIsQuery(t *testing.T) {
	cases := map[string]struct {
		payload []byte
		want    bool
	}{
		"a2s":         {[]byte{0xff, 0xff, 0xff, 0xff, 0x54}, true},
		"split reply": {[]byte{0xfe, 0xff, 0xff, 0xff, 0x01}, true},
		"too short":   {[]byte{0xff, 0xff, 0xff}, false},
		"empty":       {nil, false},
		"other":       {[]byte{0x09, 0x08, 0x00, 0x04}, false},
	}

	for name, c := range cases {
		if got := IsQuery(c.payload); got != c.want {
			t.Errorf("%s: IsQuery = %v, want %v", name, got, c.want)
		}
	}
}

func TestIgnoredSourcesNeverCount(t *testing.T) {
	d := mustNew(t, target(func(t *Target) { t.Ignore = []netip.Prefix{netip.MustParsePrefix("10.0.0.112/32")} }))

	var packets []Packet
	for i := 0; i < 10; i++ {
		packets = append(packets, pkt(time.Duration(i)*100*time.Millisecond, "10.0.0.112", 27000, []byte{1}))
	}

	if fires(d, packets...) != 0 {
		t.Fatal("traffic from an ignored source (the panel) must not trigger")
	}
}

func TestOtherPortsAreIgnored(t *testing.T) {
	d := mustNew(t, target())

	var packets []Packet
	for i := 0; i < 10; i++ {
		packets = append(packets, pkt(time.Duration(i)*100*time.Millisecond, "198.51.100.1", 9999, []byte{1}))
	}

	if fires(d, packets...) != 0 {
		t.Fatal("unwatched ports must not trigger")
	}
}

func TestHoldoffSuppressesRetriggering(t *testing.T) {
	d := mustNew(t, target())

	var packets []Packet
	for i := 0; i < 20; i++ { // a 20s attempt at 1 packet/s
		packets = append(packets, pkt(time.Duration(i)*time.Second, "198.51.100.1", 27000, []byte{1}))
	}

	if n := fires(d, packets...); n != 1 {
		t.Fatalf("one attempt should trigger once, got %d", n)
	}
}

func TestTriggersAgainAfterHoldoff(t *testing.T) {
	d := mustNew(t, target())

	var packets []Packet
	for i := 0; i < 80; i++ {
		packets = append(packets, pkt(time.Duration(i)*time.Second, "198.51.100.1", 27000, []byte{1}))
	}

	if n := fires(d, packets...); n < 2 {
		t.Fatalf("a long attempt should trigger again after the holdoff, got %d", n)
	}
}

func TestLengthRule(t *testing.T) {
	d := mustNew(t, target(func(t *Target) { t.Rule.Length = 512 }))

	wrong := make([]byte, 100)

	var packets []Packet
	for i := 0; i < 10; i++ {
		p := pkt(time.Duration(i)*100*time.Millisecond, "198.51.100.1", 27000, wrong)
		p.Length = 100
		packets = append(packets, p)
	}

	if fires(d, packets...) != 0 {
		t.Fatal("packets of the wrong length must not count")
	}

	packets = packets[:0]
	for i := 0; i < 3; i++ {
		p := pkt(time.Duration(i)*100*time.Millisecond, "198.51.100.2", 27000, wrong)
		p.Length = 512
		packets = append(packets, p)
	}

	if fires(d, packets...) != 1 {
		t.Fatal("packets of the right length must count")
	}
}

func TestPrefixRule(t *testing.T) {
	d := mustNew(t, target(func(t *Target) { t.Rule.Prefix = []byte{0x09, 0x08, 0x00} }))

	var noise []Packet
	for i := 0; i < 10; i++ {
		noise = append(noise, pkt(time.Duration(i)*100*time.Millisecond, "198.51.100.1", 27000, []byte{0x01, 0x02, 0x03}))
	}

	if fires(d, noise...) != 0 {
		t.Fatal("packets without the prefix must not count")
	}

	var join []Packet
	for i := 0; i < 3; i++ {
		join = append(join, pkt(time.Duration(i)*100*time.Millisecond, "198.51.100.2", 27000, []byte{0x09, 0x08, 0x00, byte(i)}))
	}

	if fires(d, join...) != 1 {
		t.Fatal("packets with the prefix must count")
	}
}

func TestSourceFloodIsBounded(t *testing.T) {
	d := mustNew(t, target())

	// 10000 distinct sources, one packet each, all inside the window. None may trigger and memory stays bounded.
	for i := 0; i < 10000; i++ {
		src := netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)})
		if _, ok := d.Observe(Packet{At: t0, Src: src, DstPort: 27000, Length: 1, Payload: []byte{1}}); ok {
			t.Fatal("a flood of single probes must not trigger")
		}
	}

	if n := len(d.byPort[27000].hits); n > maxSources {
		t.Fatalf("tracked %d sources, limit is %d", n, maxSources)
	}
}

func TestNewValidates(t *testing.T) {
	bad := map[string][]Target{
		"no name":      {target(func(t *Target) { t.Name = "" })},
		"no ports":     {target(func(t *Target) { t.Ports = nil })},
		"zero packets": {target(func(t *Target) { t.Rule.Packets = 0 })},
		"zero window":  {target(func(t *Target) { t.Rule.Window = 0 })},
		"shared port":  {target(), target(func(t *Target) { t.Name = "other" })},
	}

	for name, targets := range bad {
		if _, err := New(targets); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
