package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mertenvg/blade/pkg/colorterm"
)

type EnvValue struct {
	Name  string  `yaml:"name"`
	Value *string `yaml:"value,omitempty"`
}

type Service struct {
	wg        sync.WaitGroup
	cmd       *exec.Cmd
	restart   bool
	startedAt time.Time

	Name       string     `yaml:"name"`
	Watch      *Watch     `yaml:"watch"`
	InheritEnv bool       `yaml:"inheritEnv"`
	Env        []EnvValue `yaml:"env"`
	Before     string     `yaml:"before"`
	Run        string     `yaml:"run"`
	DNR        bool       `yaml:"dnr"`
	Skip       bool       `yaml:"skip"`
}

func (s *Service) Start() {
	colorterm.Info(s.Name, "starting")
	if err := s.run(s.Before); err != nil {
		fmt.Println(s.Name, "'before' cmd failed with error:", err)
		return
	}
	if err := s.start(s.Run); err != nil {
		colorterm.Error(s.Name, "failed to start with error:", err)
		return
	}
	if s.Watch != nil {
		s.Watch.Start(s.Restart)
	}
}

func (s *Service) Wait() {
	s.wg.Wait()
	colorterm.Success(s.Name, "finished")
}

func (s *Service) Restart() {
	colorterm.Info(s.Name, "restarting")

	s.wg.Add(1)
	defer s.wg.Done()

	if s.cmd != nil {
		s.restart = true
		if err := s.stop(); err != nil {
			colorterm.Error(s.Name, "couldn't stop with error:", err)
			return
		}
	} else {
		if err := s.start(s.Run); err != nil {
			colorterm.Error(s.Name, "failed to restart with error:", err)
			return
		}
	}

}

func (s *Service) Exit() {
	s.Watch.Stop()
	s.DNR = true
	colorterm.Info(s.Name, "exiting")
	if err := s.stop(); err != nil {
		colorterm.Error(s.Name, "couldn't exit with error:", err)
		return
	}
}

func (s *Service) start(cmd string) error {
	if cmd == "" {
		return nil
	}

	s.wg.Add(1)

	go func(s *Service, cmd string) {
		defer s.wg.Done()

		for {
			c := s.parse(cmd)

			if err := c.Start(); err != nil {
				colorterm.Error(s.Name, "command failed with error:", err)
				return
			}

			s.startedAt = time.Now()
			s.cmd = c

			colorterm.Success(s.Name, "running", fmt.Sprintf("(pid:%d)", s.cmd.Process.Pid))
			err := c.Wait()
			if err != nil && err.Error() != "signal: killed" {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					colorterm.Error(s.Name, "ended with error:", err)
				} else {
					colorterm.Error(s.Name, "error waiting for command:", err)
				}

			} else {
				colorterm.Info(s.Name, "ended")
			}

			if s.DNR && !s.restart {
				s.cmd = nil
				return
			}

			s.restart = false
		}
	}(s, cmd)

	return nil
}

func (s *Service) run(cmd string) error {
	if cmd == "" {
		return nil
	}

	s.wg.Add(1)
	defer s.wg.Done()

	c := s.parse(cmd)
	return c.Run()
}

func (s *Service) parse(cmd string) *exec.Cmd {
	parts := strings.Split(cmd, " ")
	name := parts[0]
	args := parts[1:]

	c := exec.Command(name, args...)
	c.Dir = "."
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if !s.InheritEnv {
		c.Env = []string{}
	}

	for _, e := range s.Env {
		v := os.Getenv(e.Name)
		if e.Value != nil {
			v = *e.Value
		}
		c.Env = append(c.Environ(), fmt.Sprintf("%s=%s", e.Name, v))
	}

	return c
}

func (s *Service) stop() error {
	if s.cmd != nil && s.cmd.Process != nil {
		if err := s.cmd.Process.Kill(); err != nil {
			return err
		}
	}
	return nil
}
