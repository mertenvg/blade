//go:build !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package main

import "os"

// infoSignals is empty on platforms without SIGINFO. signal.Notify with no
// signals is a no-op, so the status-snapshot feature is silently unavailable.
var infoSignals = []os.Signal{}
