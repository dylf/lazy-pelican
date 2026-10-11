package detector

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// The fixtures are real captures of join attempts against stopped Palworld and Valheim servers (source address
// replaced with 203.0.113.10). They are how we know the generic rule works against the games it was designed for.

type fixturePacket struct {
	MS      int64  `json:"ms"`
	Src     string `json:"src"`
	SPort   int    `json:"sport"`
	DPort   uint16 `json:"dport"`
	Len     int    `json:"len"`
	Payload string `json:"payload"`
}

func load(t *testing.T, name string) []fixturePacket {
	t.Helper()

	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}

	var rows []fixturePacket
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}

	return rows
}

func (f fixturePacket) packet(t *testing.T) Packet {
	t.Helper()

	payload, err := hex.DecodeString(f.Payload)
	if err != nil {
		t.Fatal(err)
	}

	return Packet{At: t0.Add(time.Duration(f.MS) * time.Millisecond), Src: addr(f.Src), DstPort: f.DPort, Length: f.Len, Payload: payload}
}

// attempts groups a capture into join attempts: one per source port, which is how the clients behave.
func attempts(rows []fixturePacket) map[int][]fixturePacket {
	out := map[int][]fixturePacket{}
	for _, r := range rows {
		out[r.SPort] = append(out[r.SPort], r)
	}

	return out
}

func gameTarget(name string, port uint16, mods ...func(*Target)) Target {
	t := Target{Name: name, Ports: []uint16{port}, Rule: Rule{Packets: 3, Window: 5 * time.Second, Holdoff: 30 * time.Second}}
	for _, m := range mods {
		m(&t)
	}

	return t
}

func TestReplayPalworldEveryAttemptTriggersWithinThreePackets(t *testing.T) {
	for _, file := range []string{"palworld_join1.json", "palworld_join2.json"} {
		for sport, rows := range attempts(load(t, file)) {
			d := mustNew(t, gameTarget("palworld", 27002))

			var at int
			for i, r := range rows {
				if _, ok := d.Observe(r.packet(t)); ok {
					at = i + 1

					break
				}
			}

			if at == 0 || at > 3 {
				t.Errorf("%s attempt from port %d: triggered at packet %d, want within 3", file, sport, at)
			}
		}
	}
}

func TestReplayPalworldSignatureRuleStillTriggers(t *testing.T) {
	prefix := []byte{0x09, 0x08, 0x00}

	for sport, rows := range attempts(load(t, "palworld_join2.json")) {
		d := mustNew(t, gameTarget("palworld", 27002, func(t *Target) { t.Rule.Prefix = prefix }))

		var triggered bool
		for _, r := range rows {
			if _, ok := d.Observe(r.packet(t)); ok {
				triggered = true

				break
			}
		}

		if !triggered {
			t.Errorf("attempt from port %d did not trigger with the 09 08 00 prefix rule", sport)
		}
	}
}

func TestReplayValheimGamePortTriggers(t *testing.T) {
	complete := 0

	for sport, rows := range attempts(load(t, "valheim_join.json")) {
		if rows[0].DPort != 27000 {
			continue
		}

		d := mustNew(t, gameTarget("valheim", 27000))

		var at int
		for i, r := range rows {
			if _, ok := d.Observe(r.packet(t)); ok {
				at = i + 1

				break
			}
		}

		if at == 0 || at > 3 {
			t.Errorf("attempt from port %d: triggered at packet %d, want within 3", sport, at)
		}

		complete++
	}

	if complete < 4 {
		t.Fatalf("expected at least 4 Valheim attempts in the fixture, found %d", complete)
	}
}

func TestReplayValheimSignatureRuleStillTriggers(t *testing.T) {
	for sport, rows := range attempts(load(t, "valheim_join.json")) {
		if rows[0].DPort != 27000 {
			continue
		}

		d := mustNew(t, gameTarget("valheim", 27000, func(t *Target) {
			t.Rule.Length = 512
			t.Rule.Prefix = []byte{0x20, 0x10, 0x00, 0x0d}
		}))

		var triggered bool
		for _, r := range rows {
			if _, ok := d.Observe(r.packet(t)); ok {
				triggered = true

				break
			}
		}

		if !triggered {
			t.Errorf("attempt from port %d did not trigger with the length+prefix rule", sport)
		}
	}
}

func TestReplayValheimQueryPortNeverTriggers(t *testing.T) {
	// The A2S queries the Valheim client sends to its query port must not wake a server, even watched directly.
	d := mustNew(t, gameTarget("valheim-query", 27001))

	for _, r := range load(t, "valheim_join.json") {
		if r.DPort != 27001 {
			continue
		}

		if _, ok := d.Observe(r.packet(t)); ok {
			t.Fatal("an A2S query triggered a wake")
		}
	}
}

func TestReplayWholeCaptureWithHoldoffTriggersOncePerAttemptAtMost(t *testing.T) {
	rows := load(t, "palworld_join2.json")
	d := mustNew(t, gameTarget("palworld", 27002))

	var n int
	for _, r := range rows {
		if _, ok := d.Observe(r.packet(t)); ok {
			n++
		}
	}

	if n == 0 || n > len(attempts(rows)) {
		t.Fatalf("got %d triggers for %d attempts", n, len(attempts(rows)))
	}
}
