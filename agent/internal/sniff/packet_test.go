package sniff

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ipv4UDP builds a minimal IPv4+UDP packet.
func ipv4UDP(src [4]byte, dport uint16, payload []byte, mods ...func([]byte)) []byte {
	b := make([]byte, 20+8+len(payload))
	b[0] = 0x45
	binary.BigEndian.PutUint16(b[2:], uint16(len(b)))
	b[8], b[9] = 64, 17
	copy(b[12:16], src[:])
	copy(b[16:20], []byte{10, 0, 0, 123})
	binary.BigEndian.PutUint16(b[20:], 54321)
	binary.BigEndian.PutUint16(b[22:], dport)
	binary.BigEndian.PutUint16(b[24:], uint16(8+len(payload)))
	copy(b[28:], payload)

	for _, m := range mods {
		m(b)
	}

	return b
}

func TestDecodeIPv4UDP(t *testing.T) {
	d, err := DecodeIPv4UDP(ipv4UDP([4]byte{192, 0, 2, 7}, 27002, []byte{9, 8, 0, 4}))
	if err != nil {
		t.Fatal(err)
	}

	if d.Src.String() != "192.0.2.7" || d.DstPort != 27002 || d.Length != 4 || !bytes.Equal(d.Payload, []byte{9, 8, 0, 4}) {
		t.Fatalf("unexpected decode: %+v", d)
	}
}

func TestDecodeKeepsWireLengthWhenCaptureIsTruncated(t *testing.T) {
	full := ipv4UDP([4]byte{192, 0, 2, 7}, 27000, make([]byte, 512))

	d, err := DecodeIPv4UDP(full[:100])
	if err != nil {
		t.Fatal(err)
	}

	if d.Length != 512 || len(d.Payload) != 72 {
		t.Fatalf("length=%d payload=%d", d.Length, len(d.Payload))
	}
}

func TestDecodeIgnoresTrailingPadding(t *testing.T) {
	b := append(ipv4UDP([4]byte{192, 0, 2, 7}, 1, []byte{1, 2}), 0, 0, 0, 0) // ethernet padding

	d, err := DecodeIPv4UDP(b)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(d.Payload, []byte{1, 2}) {
		t.Fatalf("payload %v", d.Payload)
	}
}

func TestDecodeRejects(t *testing.T) {
	good := func() []byte { return ipv4UDP([4]byte{192, 0, 2, 7}, 1, []byte{1}) }

	cases := map[string][]byte{
		"empty":           nil,
		"too short":       good()[:19],
		"ipv6":            ipv4UDP([4]byte{}, 1, nil, func(b []byte) { b[0] = 0x65 }),
		"tcp":             ipv4UDP([4]byte{}, 1, []byte{1}, func(b []byte) { b[9] = 6 }),
		"later fragment":  ipv4UDP([4]byte{}, 1, []byte{1}, func(b []byte) { binary.BigEndian.PutUint16(b[6:], 100) }),
		"bad ihl":         ipv4UDP([4]byte{}, 1, []byte{1}, func(b []byte) { b[0] = 0x41 }),
		"udp len too low": ipv4UDP([4]byte{}, 1, []byte{1}, func(b []byte) { binary.BigEndian.PutUint16(b[24:], 3) }),
		"no udp header":   good()[:24],
	}

	for name, b := range cases {
		if _, err := DecodeIPv4UDP(b); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDecodeHonoursIPOptions(t *testing.T) {
	// IHL=6 (24 byte header): the UDP header starts 4 bytes later.
	b := make([]byte, 24+8+2)
	b[0] = 0x46
	b[9] = 17
	copy(b[12:16], []byte{192, 0, 2, 7})
	binary.BigEndian.PutUint16(b[26:], 27000)
	binary.BigEndian.PutUint16(b[28:], 10)
	copy(b[32:], []byte{0xaa, 0xbb})

	d, err := DecodeIPv4UDP(b)
	if err != nil {
		t.Fatal(err)
	}

	if d.DstPort != 27000 || !bytes.Equal(d.Payload, []byte{0xaa, 0xbb}) {
		t.Fatalf("unexpected decode: %+v", d)
	}
}
