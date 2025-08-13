package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/mertenvg/blade/pkg/blade"
)

func main() {
	defer blade.Done()

	fmt.Println("hi, i am service-one")

	// Wait for interrupt or term signal from OS
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
	fmt.Println("service-one shutting down...")
}
