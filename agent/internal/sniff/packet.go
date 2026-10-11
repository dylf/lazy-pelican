// Package sniff captures inbound UDP packets and decodes them for the detector.
package sniff

import (
	"encoding/binary"
	"errors"
	"net/netip"
)

// Decoded is the part of an IPv4/UDP datagram the agent cares about.
type Decoded struct {
	Src     netip.Addr
	DstPort uint16
	Length  int
	Payload []byte
}

var errNotUDP = errors.New("not an IPv4 UDP datagram")

// DecodeIPv4UDP parses a packet that starts at the IPv4 header (no link-layer header).
func DecodeIPv4UDP(b []byte) (Decoded, error) {
	if len(b) < 20 || b[0]>>4 != 4 {
		return Decoded{}, errNotUDP
	}

	ihl := int(b[0]&0x0f) * 4
	if ihl < 20 || len(b) < ihl+8 || b[9] != 17 {
		return Decoded{}, errNotUDP
	}

	// Only the first fragment carries the UDP header.
	if binary.BigEndian.Uint16(b[6:8])&0x1fff != 0 {
		return Decoded{}, errNotUDP
	}

	udpLen := int(binary.BigEndian.Uint16(b[ihl+4 : ihl+6]))
	if udpLen < 8 {
		return Decoded{}, errNotUDP
	}

	payload := b[ihl+8:]
	if want := udpLen - 8; want < len(payload) {
		payload = payload[:want]
	}

	src, _ := netip.AddrFromSlice(b[12:16])

	return Decoded{
		Src:     src,
		DstPort: binary.BigEndian.Uint16(b[ihl+2 : ihl+4]),
		Length:  udpLen - 8,
		Payload: payload,
	}, nil
}
