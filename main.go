package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"gopkg.in/yaml.v3"
)

func main() {
	b, err := os.ReadFile("./blade.yaml")
	if err != nil {
		fmt.Println("Couldn't load config file blade.yaml", err)
		os.Exit(1)
	}

	var conf []*Service

	err = yaml.Unmarshal(b, &conf)
	if err != nil {
		fmt.Println("Couldn't parse yaml from blade.yaml", err)
		os.Exit(1)
	}

	services := make(map[string]*Service)
	for _, s := range conf {
		services[s.Name] = s
	}

	var wg sync.WaitGroup

	args := os.Args
	if len(args) > 1 {

		action := args[1]
		switch action {
		case "run":
			var run []*Service
			if len(args) > 2 {
				for _, name := range args[2:] {
					s, ok := services[name]
					if !ok {
						fmt.Println("Couldn't find service", name)
						os.Exit(1)
					}
					run = append(run, s)
				}
			} else {
				for _, s := range conf {
					run = append(run, s)
				}
			}

			for _, s := range run {
				wg.Add(1)
				s.Start()
				go func(s *Service) {
					s.Wait()
					wg.Done()
				}(s)
			}
		}

	} else {
		fmt.Println("Services available:")
		for _, s := range conf {
			fmt.Println(" -", s.Name)
		}
		fmt.Println("Usage: blade run")
		fmt.Println("Or: blade run <service-name-1> <service-name-2> <service-name-n>...")
	}

	// Wait for interrupt or term signal from OS
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()
	fmt.Println("shutting down...")

	for _, s := range conf {
		s.Stop()
	}

	wg.Wait()
}
