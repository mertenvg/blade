package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mertenvg/blade/pkg/blade"
)

func main() {
	defer blade.Done()

	fmt.Println("hi, i am update-loop")

	pid := os.Getpid()
	ppid := os.Getppid()
	fmt.Println("update-loop pid:", pid, "and parent pid:", ppid)

	defer func() {
		fmt.Println("update-loop deferred goodbye")
	}()

	// Wait for interrupt or term signal from OS
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINFO)

	done := make(chan struct{}, 1)

	go func() {
		fmt.Println("update-loop waiting for signal")
		defer func() {
			fmt.Println("update-loop done")
			close(done)
		}()

		for {
			select {
			case <-time.After(time.Second): // time.Duration(rand.Int64N(15)+1)
				err := os.WriteFile("./cmd/update-loop/time.txt", []byte(time.Now().String()), os.ModePerm)
				fmt.Println("update-loop file updated")
				if err != nil {
					fmt.Println(err)
				}
			case <-ctx.Done():
				fmt.Println("update-loop context done")
				return
			case s := <-sig:
				fmt.Println("update-loop got signal", s)
			}
		}
	}()

	<-done

	fmt.Println("update-loop shutting down...")
}
