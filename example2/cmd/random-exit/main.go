package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os/signal"
	"syscall"
	"time"

	"github.com/mertenvg/blade/pkg/blade"
)

func main() {
	defer blade.Done()

	fmt.Println("hi, i am random-exit")

	// Wait for interrupt or term signal from OS
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	select {
	case <-ctx.Done():
		break
	case <-time.After(time.Second * time.Duration(rand.Int64N(10)+1)):
		break
	}

	fmt.Println("random-exit shutting down...")
}
