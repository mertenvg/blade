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
	err := os.WriteFile(fmt.Sprintf("./%s.pid", serviceName), []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	if err != nil {
		panic(fmt.Sprintf("failed to write pid file: %s", err))
	}
}

func Done() {
	if serviceName == "" {
		return
	}
	err := os.Remove(fmt.Sprintf("./%s.pid", serviceName))
	if err != nil {
		panic(fmt.Sprintf("failed to remove pid file: %s", err))
	}
}
