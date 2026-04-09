//go:build !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package main

import "os"

// sendInfoSignal is a no-op on platforms without SIGINFO.
func sendInfoSignal(p *os.Process) error { return nil }
