package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	defer cancel()

	done := make(chan struct{}, 1)

	cmdCtx, cmdCancel := context.WithCancel(context.Background())

	go func() {
		defer close(done)

		parts := strings.Split("go run cmd/update-loop/main.go", " ")
		name := parts[0]
		args := parts[1:]

		c := exec.CommandContext(cmdCtx, name, args...)
		c.Dir = "."
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Start(); err != nil {
			fmt.Println("test-update-loop error starting command:", err)
		}

		fmt.Println("test-update-loop started update-loop with pid", c.Process.Pid)

		if err := c.Process.Signal(syscall.SIGINFO); err != nil {
			fmt.Println("test-update-loop error sending info signal:", err)
		}

		if err := c.Wait(); err != nil {
			fmt.Println("test-update-loop error waiting:", err)
		}
	}()

	<-time.After(60 * time.Second)

	cmdCancel()

	<-done

	fmt.Println("test-update-loop done")

	<-ctx.Done()

	fmt.Println("test-update-loop shutting down...")
}
