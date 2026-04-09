//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func notifyInfoSignal(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGINFO)
}
