package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mertenvg/blade/internal/service/watcher"
	"github.com/mertenvg/blade/pkg/colorterm"
)

type EnvValue struct {
	Name  string  `yaml:"name"`
	Value *string `yaml:"value,omitempty"`
}

type Output struct {
	Stdout string `yaml:"stdout"`
	Stderr string `yaml:"stderr"`
	Stdin  string `yaml:"stdin"`
}

type S struct {
	wg        sync.WaitGroup
	cancel    context.CancelFunc
	startedAt time.Time
	backoff   int
	state     string
	pid       int

	Name       string     `yaml:"name"`
	Watch      *watcher.W `yaml:"watch"`
	InheritEnv bool       `yaml:"inheritEnv"`
	Env        []EnvValue `yaml:"env"`
	Before     string     `yaml:"before"`
	Run        string     `yaml:"run"`
	DNR        bool       `yaml:"dnr"`
	Skip       bool       `yaml:"skip"`
	Dir        string     `yaml:"dir"`
	Output     Output     `yaml:"output"`
	Sleep      int        `yaml:"sleep"`
}

func (s *S) Start() {
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

func (s *S) Wait() {
	s.wg.Wait()
	colorterm.Success(s.Name, "finished")
}

func (s *S) Restart() {
	colorterm.Info(s.Name, "restarting")

	s.wg.Add(1)
	defer s.wg.Done()

	s.stop()
}

func (s *S) Exit() {
	s.Watch.Stop()
	s.DNR = true
	colorterm.Info(s.Name, "exiting")
	s.stop()
}

func (s *S) Status() (bool, string, string) {
	active := false
	state := "not running"
	pid := "()"
	if s.state != "" {
		state = s.state
	}
	if s.pid != 0 {
		pid = fmt.Sprintf("(%d)", s.pid)
		p := s.process()
		if p != nil {
			err := p.Signal(syscall.Signal(0))
			if err == nil {
				state = fmt.Sprintf("OK %s", time.Now().Sub(s.startedAt).Round(time.Second).String())
				active = true
			}
			if err != nil {
				state = err.Error()
			}
		}
	}
	return active, state, pid
}

func (s *S) start(cmd string) error {
	if cmd == "" {
		return fmt.Errorf("cmd is empty")
	}

	s.wg.Add(1)

	go func(s *S, cmd string) {
		defer s.wg.Done()

		for {
			c, cancel := s.parse(cmd)

			s.cancel = cancel
			s.startedAt = time.Now()

			if err := c.Start(); err != nil {
				cancel()

				colorterm.Error(s.Name, "command failed with error:", err)

				if s.DNR {
					return
				}

				if s.backoff == 0 {
					s.backoff = 1
				}

				time.Sleep(time.Second * time.Duration(s.backoff))
				s.backoff *= 2

				continue
			}

			s.backoff = 0
			s.pid = s.getpid(c.Process.Pid)

			colorterm.Success(s.Name, "running", fmt.Sprintf("(pid:%d)", s.pid))

			p := s.process()

			ps, err := p.Wait()
			if err != nil && err.Error() != "signal: killed" && err.Error() != "wait: no child processes" {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					colorterm.Error(s.Name, "ended with error:", err)
				} else {
					colorterm.Error(s.Name, "error waiting for process:", err)
				}
			} else {
				colorterm.Info(s.Name, "ended")
			}

			// this may be the same process we've waited for above, or a parent process.
			_ = c.Wait()

			state := ps.String()
			if ps == nil {
				state = fmt.Sprintf("OK %s", time.Now().Sub(s.startedAt).Round(time.Second))
			}
			s.state = state

			if s.DNR {
				return
			}

			if s.Sleep > 0 {
				time.Sleep(time.Duration(s.Sleep) * time.Millisecond)
			}
		}
	}(s, cmd)

	return nil
}

func (s *S) run(cmd string) error {
	if cmd == "" {
		return nil
	}

	s.wg.Add(1)
	defer s.wg.Done()

	c, cancel := s.parse(cmd)
	defer cancel()

	s.cancel = cancel

	return c.Run()
}

func (s *S) parse(cmd string) (*exec.Cmd, context.CancelFunc) {
	parts := strings.Split(cmd, " ")
	name := parts[0]
	args := parts[1:]

	ctx, cancel := context.WithCancel(context.Background())

	c := exec.CommandContext(ctx, name, args...)
	c.Dir = coalesce(s.Dir, ".")

	if s.Output.Stdout == "os" {
		c.Stdout = os.Stdout
	}

	if s.Output.Stderr == "os" {
		c.Stderr = os.Stderr
	}

	if s.Output.Stdin != "os" {
		c.Stdin = os.Stdin
	}

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

	c.Env = append(c.Environ(), fmt.Sprintf("BLADE_SERVICE_NAME=%s", s.Name))

	return c, cancel
}

func (s *S) stop() {
	if s.cancel != nil {
		s.cancel()
	}
	proc := s.process()
	if proc != nil {
		if err := proc.Kill(); err != nil {
			colorterm.Error(s.Name, "failed to kill process:", err)
		}
	}
}

func (s *S) process() *os.Process {
	if s.pid > 0 {
		proc, err := os.FindProcess(s.pid)
		if err != nil {
			colorterm.Error(s.Name, "failed to find process:", err)
		}
		return proc
	}
	return nil
}

func (s *S) getpid(assume int) int {
	pidFilePath := filepath.Join(s.Dir, fmt.Sprintf(".%s.pid", s.Name))
	_, err := os.Stat(pidFilePath)
	if err != nil {
		return assume
	}
	text, err := os.ReadFile(pidFilePath)
	if err != nil {
		return assume
	}
	pid, err := strconv.Atoi(string(text))
	if err != nil {
		return assume
	}
	return pid
}

func coalesce(args ...string) string {
	for _, str := range args {
		if str != "" {
			return str
		}
	}
	return ""
}
