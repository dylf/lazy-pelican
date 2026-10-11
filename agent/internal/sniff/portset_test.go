package sniff

import (
	"sync"
	"testing"
)

func TestPortSet(t *testing.T) {
	s := NewPortSet(27000, 27002)

	if !s.Has(27000) || !s.Has(27002) || s.Has(27001) {
		t.Fatal("unexpected membership")
	}

	s.Set([]uint16{27001})

	if s.Has(27000) || !s.Has(27001) {
		t.Fatal("Set must replace the whole set")
	}

	s.Set(nil)

	if s.Has(27001) {
		t.Fatal("an empty set watches nothing")
	}
}

func TestZeroPortSetWatchesNothing(t *testing.T) {
	var s PortSet

	if s.Has(1) {
		t.Fatal("zero value must be empty")
	}
}

func TestPortSetIsSafeForConcurrentUse(t *testing.T) {
	s := NewPortSet(1)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()

			for j := 0; j < 1000; j++ {
				s.Set([]uint16{uint16(j)})
			}
		}()

		go func() {
			defer wg.Done()

			for j := 0; j < 1000; j++ {
				s.Has(uint16(j))
			}
		}()
	}

	wg.Wait()
}
