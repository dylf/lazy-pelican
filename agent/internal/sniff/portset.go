package sniff

import "sync/atomic"

// PortSet is the set of destination ports to capture. It can be replaced while capture is running, because the
// panel can change which servers to watch at any time.
type PortSet struct {
	ports atomic.Pointer[map[uint16]struct{}]
}

func NewPortSet(ports ...uint16) *PortSet {
	s := &PortSet{}
	s.Set(ports)

	return s
}

func (s *PortSet) Set(ports []uint16) {
	m := make(map[uint16]struct{}, len(ports))
	for _, p := range ports {
		m[p] = struct{}{}
	}

	s.ports.Store(&m)
}

func (s *PortSet) Has(port uint16) bool {
	m := s.ports.Load()
	if m == nil {
		return false
	}

	_, ok := (*m)[port]

	return ok
}
