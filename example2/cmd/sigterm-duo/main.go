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

	fmt.Println("hi, i am sigterm-duo")

	// Wait for interrupt or term signal from OS
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()

	// Wait for interrupt or term signal from OS once more
	ctx2, cancel2 := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel2()
	<-ctx2.Done()

	fmt.Println("sigterm-duo shutting down...")
}
