//go:build !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package main

import "os"

// notifyInfoSignal is a no-op on platforms without SIGINFO.
func notifyInfoSignal(ch chan<- os.Signal) {}
