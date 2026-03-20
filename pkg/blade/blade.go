package blade

import (
	"fmt"
	"os"
)

var serviceName = os.Getenv("BLADE_SERVICE_NAME")

func init() {
	if serviceName == "" {
		return
	}
	err := os.WriteFile(fmt.Sprintf("./.%s.pid", serviceName), []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	if err != nil {
		panic(fmt.Sprintf("failed to write pid file: %s", err))
	}
}

func Done() {
	if serviceName == "" {
		return
	}

	pidFile := fmt.Sprintf("./.%s.pid", serviceName)
	myPid := fmt.Sprintf("%d", os.Getpid())

	contents, err := os.ReadFile(pidFile)
	if err != nil {
		// File already gone or unreadable — nothing to clean up
		return
	}

	if string(contents) != myPid {
		// File contains a different PID (newer process already took over) — leave it alone
		return
	}

	_ = os.Remove(pidFile)
}
