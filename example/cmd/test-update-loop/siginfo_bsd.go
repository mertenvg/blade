//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package main

import (
	"os"
	"syscall"
)

func sendInfoSignal(p *os.Process) error {
	return p.Signal(syscall.SIGINFO)
}
