package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

type EnvValue struct {
	Name  string  `yaml:"name"`
	Value *string `yaml:"value,omitempty"`
}

type Service struct {
	wg      sync.WaitGroup
	cmd     *exec.Cmd
	restart bool

	Name       string     `yaml:"name"`
	Watch      *Watch     `yaml:"watch"`
	InheritEnv bool       `yaml:"inheritEnv"`
	Env        []EnvValue `yaml:"env"`
	Before     string     `yaml:"before"`
	Run        string     `yaml:"run"`
	DNR        bool       `yaml:"dnr"`
}

func (s *Service) Start() {
	fmt.Println("starting service", s.Name)
	if err := s.run(s.Before); err != nil {
		fmt.Println("couldn't run cmd", s.Before, s.Name, err)
		return
	}
	if err := s.start(s.Run); err != nil {
		fmt.Println("couldn't start service", s.Name, err)
		return
	}
	if s.Watch != nil {
		s.Watch.Start(s.Restart)
	}
}

func (s *Service) Wait() {
	fmt.Println("waiting for service", s.Name)
	s.wg.Wait()
	fmt.Println("service", s.Name, "ended")
}

func (s *Service) Restart() {
	fmt.Println("refreshing service", s.Name)

	s.wg.Add(1)
	defer s.wg.Done()

	s.restart = true
	if err := s.stop(syscall.SIGTERM); err != nil {
		fmt.Println("couldn't stop service", s.Name, err)
		return
	}
	if err := s.start(s.Run); err != nil {
		fmt.Println("couldn't start service", s.Name, err)
		return
	}
}

func (s *Service) Stop() {
	s.Watch.Stop()
	s.DNR = true
	fmt.Printf("stopping service %s\n", s.Name)
	if err := s.stop(syscall.SIGTERM); err != nil {
		fmt.Println("couldn't stop service", s.Name, err)
		return
	}
}

func (s *Service) start(cmd string) error {
	if cmd == "" {
		return nil
	}

	c := s.parse(cmd)

	go func(s *Service, c *exec.Cmd) {
		s.wg.Add(1)
		defer s.wg.Done()

		for {
			if err := c.Start(); err != nil {
				fmt.Println("couldn't start service", s.Name, err)
				return
			}

			s.restart = false

			err := c.Wait()
			fmt.Println("service", s.Name, err)

			if s.DNR || s.restart {
				return
			}
		}
	}(s, c)

	s.cmd = c

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
	c.Stdin = os.Stdin

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

func (s *Service) stop(sig syscall.Signal) error {
	if s.cmd != nil {
		if err := s.cmd.Process.Signal(sig); err != nil {
			return err
		}
		s.cmd = nil
	}
	return nil
}
