package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mertenvg/blade/pkg/colorterm"
)

func main() {
	b, err := os.ReadFile("./blade.yaml")
	if err != nil {
		colorterm.Error("Couldn't load config file blade.yaml", err)
		os.Exit(1)
	}

	var conf []*Service

	err = yaml.Unmarshal(b, &conf)
	if err != nil {
		colorterm.Error("Couldn't parse yaml from blade.yaml", err)
		os.Exit(1)
	}

	services := make(map[string]*Service)
	for _, s := range conf {
		services[s.Name] = s
	}

	var wg sync.WaitGroup

	defer func() {
		if r := recover(); r != nil {
			colorterm.Error(r)
			for _, s := range conf {
				s.Exit()
			}
			os.Exit(1)
		}
	}()

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
						colorterm.Error("Couldn't find service", name)
						os.Exit(1)
					}
					run = append(run, s)
				}
			} else {
				for _, s := range conf {
					if s.Skip {
						colorterm.Debug(s.Name, "skipping")
						continue
					}
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
		colorterm.Info("Services available:")
		for _, s := range conf {
			colorterm.Info(" -", s.Name)
		}
		colorterm.None("Usage: blade run")
		colorterm.None("Or: blade run <service-name-1> <service-name-2> <service-name-n>...")
		return
	}

	info := make(chan os.Signal, 1)
	signal.Notify(info, syscall.SIGINFO)

	// Wait for interrupt or term signal from OS
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	done := make(chan struct{}, 1)

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-info:
				for _, s := range conf {
					state := "not running"
					pid := "()"
					if s.cmd != nil {
						state = s.cmd.ProcessState.String()
						pid = fmt.Sprintf("(%d)", s.cmd.Process.Pid)
					}
					if state != "<nil>" {
						colorterm.Error(s.Name, pid, state)
					} else {
						state = fmt.Sprintf("OK %s", time.Now().Sub(s.startedAt).Round(time.Second))
						colorterm.Success(s.Name, pid, state)
					}
				}
			}
		}
	}()

	<-done

	close(info)

	colorterm.Warning("shutting down...")

	for _, s := range conf {
		s.Exit()
	}

	wg.Wait()
}
