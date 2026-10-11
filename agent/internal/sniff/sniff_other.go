//go:build !linux

package sniff

import (
	"context"
	"errors"

	"github.com/dylf/lazy-pelican/agent/internal/detector"
)

var errUnsupported = errors.New("packet capture is only supported on Linux")

func Check(string) error { return errUnsupported }

func Run(context.Context, string, []uint16, chan<- detector.Packet) error { return errUnsupported }
