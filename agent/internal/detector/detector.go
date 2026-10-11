// Package detector decides when UDP traffic to a stopped game server looks like a player trying to join.
//
// It is deliberately game-agnostic. Steam "A2S" queries (packets starting FF FF FF FF) are what scanners, server
// browsers and reflection attacks send, so they are ignored. What is left is counted per source address: a
// player's client keeps retrying for several seconds, while a scanner typically sends one probe. Optional
// per-server match rules (payload prefix, exact length) tighten this for games that are known to be noisy.
package detector

import (
	"bytes"
	"fmt"
	"net/netip"
	"time"
)

// Packet is one observed inbound UDP datagram.
type Packet struct {
	At      time.Time
	Src     netip.Addr
	DstPort uint16
	Length  int    // UDP payload length on the wire
	Payload []byte // may be truncated by the capture
}

// Rule controls when a burst of packets counts as a join attempt.
type Rule struct {
	Packets int           // packets from one source needed to trigger
	Window  time.Duration // ...within this long
	Holdoff time.Duration // after a trigger, ignore the server for this long

	// Optional narrowing. Zero values mean "any".
	Prefix []byte
	Length int
}

// Target is a server to watch.
type Target struct {
	Name   string
	Ports  []uint16
	Rule   Rule
	Ignore []netip.Prefix // sources that never count (e.g. the panel)
}

// maxSources bounds memory when something floods the port from many addresses; past it new sources are ignored.
const maxSources = 4096

type state struct {
	Target
	hits      map[netip.Addr][]time.Time
	holdUntil time.Time
}

// Detector is not safe for concurrent use; feed it from a single goroutine.
type Detector struct {
	byPort map[uint16]*state
}

// New validates the targets and builds a Detector.
func New(targets []Target) (*Detector, error) {
	d := &Detector{byPort: map[uint16]*state{}}

	for _, t := range targets {
		if t.Name == "" {
			return nil, fmt.Errorf("target without a name")
		}
		if len(t.Ports) == 0 {
			return nil, fmt.Errorf("target %q has no ports", t.Name)
		}
		if t.Rule.Packets < 1 || t.Rule.Window <= 0 {
			return nil, fmt.Errorf("target %q needs packets >= 1 and a positive window", t.Name)
		}

		s := &state{Target: t, hits: map[netip.Addr][]time.Time{}}
		for _, port := range t.Ports {
			if _, taken := d.byPort[port]; taken {
				return nil, fmt.Errorf("port %d is used by more than one target", port)
			}
			d.byPort[port] = s
		}
	}

	return d, nil
}

// Ports lists every port being watched.
func (d *Detector) Ports() []uint16 {
	ports := make([]uint16, 0, len(d.byPort))
	for p := range d.byPort {
		ports = append(ports, p)
	}

	return ports
}

// IsQuery reports whether the payload is a Steam server query or reply (FF FF FF FF / FE FF FF FF header).
func IsQuery(payload []byte) bool {
	return len(payload) >= 4 && (bytes.HasPrefix(payload, []byte{0xff, 0xff, 0xff, 0xff}) || bytes.HasPrefix(payload, []byte{0xfe, 0xff, 0xff, 0xff}))
}

// Observe feeds one packet in and returns the target's name when it looks like a join attempt.
func (d *Detector) Observe(p Packet) (string, bool) {
	s, ok := d.byPort[p.DstPort]
	if !ok {
		return "", false
	}

	if p.At.Before(s.holdUntil) || IsQuery(p.Payload) {
		return "", false
	}

	for _, prefix := range s.Ignore {
		if prefix.Contains(p.Src) {
			return "", false
		}
	}

	if s.Rule.Length != 0 && p.Length != s.Rule.Length {
		return "", false
	}

	if len(s.Rule.Prefix) > 0 && !bytes.HasPrefix(p.Payload, s.Rule.Prefix) {
		return "", false
	}

	if _, known := s.hits[p.Src]; !known && len(s.hits) >= maxSources {
		s.sweep(p.At)

		if len(s.hits) >= maxSources {
			return "", false
		}
	}

	cutoff := p.At.Add(-s.Rule.Window)
	times := s.hits[p.Src][:0]
	for _, t := range s.hits[p.Src] {
		if t.After(cutoff) {
			times = append(times, t)
		}
	}
	times = append(times, p.At)

	if len(times) < s.Rule.Packets {
		s.hits[p.Src] = times

		return "", false
	}

	s.hits = map[netip.Addr][]time.Time{}
	s.holdUntil = p.At.Add(s.Rule.Holdoff)

	return s.Name, true
}

func (s *state) sweep(now time.Time) {
	cutoff := now.Add(-s.Rule.Window)

	for src, times := range s.hits {
		if len(times) == 0 || !times[len(times)-1].After(cutoff) {
			delete(s.hits, src)
		}
	}
}
