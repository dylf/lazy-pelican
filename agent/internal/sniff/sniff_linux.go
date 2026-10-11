//go:build linux

package sniff

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/dylf/lazy-pelican/agent/internal/detector"
)

const (
	ethPIP         = 0x0800
	packetOutgoing = 4
)

func htons(v uint16) uint16 { return v<<8 | v>>8 }

func open(iface string) (int, error) {
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_DGRAM, int(htons(ethPIP)))
	if err != nil {
		return -1, fmt.Errorf("opening packet socket (needs CAP_NET_RAW or root): %w", err)
	}

	if iface != "" && iface != "any" {
		ifc, err := net.InterfaceByName(iface)
		if err != nil {
			syscall.Close(fd)

			return -1, fmt.Errorf("interface %q: %w", iface, err)
		}

		if err := syscall.Bind(fd, &syscall.SockaddrLinklayer{Protocol: htons(ethPIP), Ifindex: ifc.Index}); err != nil {
			syscall.Close(fd)

			return -1, fmt.Errorf("binding to %q: %w", iface, err)
		}
	}

	// Wake up regularly so a cancelled context is noticed.
	tv := syscall.NsecToTimeval(int64(time.Second))
	if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); err != nil {
		syscall.Close(fd)

		return -1, err
	}

	return fd, nil
}

// Check verifies that a packet socket can be opened, for the doctor command.
func Check(iface string) error {
	fd, err := open(iface)
	if err != nil {
		return err
	}

	return syscall.Close(fd)
}

// Run reads inbound IPv4/UDP packets destined for any of the ports in the set and sends them on out until ctx is
// cancelled. ready, if given, is called once the capture socket is open.
//
// Filtering happens in userspace, so every inbound IPv4 packet on the interface is copied to the agent. That is
// fine for a game host, but a BPF filter would be cheaper on a busy one.
func Run(ctx context.Context, iface string, ports *PortSet, out chan<- detector.Packet, ready func()) error {
	fd, err := open(iface)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)

	if ready != nil {
		ready()
	}

	buf := make([]byte, 65536)

	for ctx.Err() == nil {
		n, from, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
				continue
			}

			return fmt.Errorf("reading packets: %w", err)
		}

		if ll, ok := from.(*syscall.SockaddrLinklayer); ok && ll.Pkttype == packetOutgoing {
			continue
		}

		d, err := DecodeIPv4UDP(buf[:n])
		if err != nil || !ports.Has(d.DstPort) {
			continue
		}

		select {
		case out <- detector.Packet{At: time.Now(), Src: d.Src, DstPort: d.DstPort, Length: d.Length, Payload: append([]byte(nil), d.Payload...)}:
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}
