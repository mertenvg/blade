//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package main

import (
	"os"
	"syscall"
)

// infoSignals are the signals that trigger a status snapshot. SIGINFO is only
// available on BSD-derived systems (macOS, *BSD); on other platforms this
// degrades to an empty list and the feature is silently disabled.
var infoSignals = []os.Signal{syscall.SIGINFO}
