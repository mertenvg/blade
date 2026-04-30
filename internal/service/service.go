package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mertenvg/blade/pkg/coalesce"

	"github.com/mertenvg/blade/internal/service/watcher"
	"github.com/mertenvg/blade/pkg/colorterm"
)

// empty is used for signal-only channels in place of struct{}{}.
type empty struct{}

// gracePeriod is how long we wait between SIGTERM and SIGKILL when
// cancelling or restarting a running child process.
const gracePeriod = 5 * time.Second

type EnvValue struct {
	Name  string  `yaml:"name"`
	Value *string `yaml:"value,omitempty"`
}

type Output struct {
	Stdout string `yaml:"stdout"`
	Stderr string `yaml:"stderr"`
	Stdin  string `yaml:"stdin"`
}

func (o Output) InheritFrom(parent Output) Output {
	return Output{
		Stdout: coalesce.String(parent.Stdout, o.Stdout),
		Stderr: coalesce.String(parent.Stderr, o.Stderr),
		Stdin:  coalesce.String(parent.Stdin, o.Stdin),
	}
}

type S struct {
	wg        sync.WaitGroup
	restartCh chan empty
	startedAt time.Time
	backoff   time.Duration
	state string
	pid   int

	Name       string     `yaml:"name"`
	From       string     `yaml:"from"`
	Tags       []string   `yaml:"tags"`
	Watch      *watcher.W `yaml:"watch"`
	InheritEnv bool       `yaml:"inheritEnv"`
	Env        []EnvValue `yaml:"env"`
	Once       string     `yaml:"once"`
	Before     string     `yaml:"before"`
	Run        string     `yaml:"run"`
	DNR        bool       `yaml:"dnr"`
	Skip       bool       `yaml:"skip"`
	Dir        string     `yaml:"dir"`
	Output     Output     `yaml:"output"`
	Sleep      int        `yaml:"sleep"`
}

func (s *S) Start(ctx context.Context) {
	colorterm.Info(s.Name, "starting")
	if err := s.run(ctx, s.Once); err != nil {
		fmt.Println(s.Name, "'once' cmd failed with error:", err)
		return
	}
	s.restartCh = make(chan empty, 1)
	if err := s.start(ctx, s.Run); err != nil {
		colorterm.Error(s.Name, "failed to start with error:", err)
		return
	}
	if s.Watch != nil {
		s.Watch.Start(ctx, s.Restart)
	}
}

func (s *S) Wait() {
	s.wg.Wait()
	colorterm.Success(s.Name, "finished")
}

// Restart asks the running loop to terminate the current child process and
// start a new one. It is a non-blocking signal; coalesces if already pending.
func (s *S) Restart() {
	colorterm.Info(s.Name, "restarting")
	if s.restartCh == nil {
		return
	}
	select {
	case s.restartCh <- empty{}:
	default:
	}
}

// Exit marks the service as do-not-restart and wakes the run loop so it can
// observe the flag and return. Safe to call even if the service was never
// started or has no watcher.
func (s *S) Exit() {
	if s.Watch != nil {
		s.Watch.Stop()
	}
	s.DNR = true
	colorterm.Info(s.Name, "exiting")
	s.Restart()
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
		p, _ := os.FindProcess(s.pid)
		if p != nil {
			err := p.Signal(syscall.Signal(0))
			if err == nil {
				state = fmt.Sprintf("OK %s", time.Since(s.startedAt).Round(time.Second).String())
				active = true
			}
			if err != nil {
				state = err.Error()
			}
		}
	}
	return active, state, pid
}

func (s *S) InheritFrom(parent *S) {
	// Allocate fresh backing arrays so later mutations to s.Tags / s.Env
	// (e.g. main.go rewriting Env[i].Value during interpolation) cannot
	// leak back into parent and corrupt other siblings that inherit from it.
	tags := make([]string, 0, len(parent.Tags)+len(s.Tags)) // []string   `yaml:"tags"`
	tags = append(tags, parent.Tags...)
	tags = append(tags, s.Tags...)
	s.Tags = tags

	env := make([]EnvValue, 0, len(parent.Env)+len(s.Env)) // []EnvValue `yaml:"env"`
	env = append(env, parent.Env...)
	env = append(env, s.Env...)
	s.Env = env

	s.Once = coalesce.String(parent.Once, s.Once)       // string     `yaml:"once"`
	s.Before = coalesce.String(parent.Before, s.Before) // string     `yaml:"before"`
	s.Run = coalesce.String(parent.Run, s.Run)          // string     `yaml:"run"`
	s.Dir = coalesce.String(parent.Dir, s.Dir)          // string     `yaml:"dir"`
	s.Sleep = coalesce.Int(parent.Sleep, s.Sleep)       // int        `yaml:"sleep"`

	s.InheritEnv = parent.InheritEnv || s.InheritEnv // bool       `yaml:"inheritEnv"`
	s.DNR = parent.DNR || s.DNR                      // bool       `yaml:"dnr"`
	s.Skip = parent.Skip || s.Skip                   // bool       `yaml:"skip"`

	s.Output = s.Output.InheritFrom(parent.Output)
	s.Watch = s.Watch.InheritFrom(parent.Watch)
}

func (s *S) start(ctx context.Context, cmd string) error {
	if cmd == "" {
		return fmt.Errorf("cmd is empty")
	}

	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		for {
			if ctx.Err() != nil || s.DNR {
				return
			}

			if err := s.run(ctx, s.Before); err != nil {
				colorterm.Error(s.Name, "'before' cmd failed with error:", err)

				if s.DNR || ctx.Err() != nil {
					return
				}

				if s.backoff == 0 {
					s.backoff = time.Second
				}
				if !sleepCtx(ctx, s.backoff) {
					return
				}
				s.backoff *= 2

				continue
			}

			cmdCtx, cmdCancel := context.WithCancel(ctx)
			c, closeOutputs := s.parse(cmdCtx, cmd)

			s.startedAt = time.Now()

			if err := c.Start(); err != nil {
				closeOutputs()
				cmdCancel()

				colorterm.Error(s.Name, "command failed with error:", err)

				if s.DNR || ctx.Err() != nil {
					return
				}

				if s.backoff == 0 {
					s.backoff = time.Second
				}
				if !sleepCtx(ctx, s.backoff) {
					return
				}
				s.backoff *= 2

				continue
			}

			s.pid = c.Process.Pid
			colorterm.Success(s.Name, "running", fmt.Sprintf("(pid:%d)", s.pid))

			restartRequested := s.waitCmd(ctx, c)

			closeOutputs()
			cmdCancel()

			s.waitForExit(ctx)
			s.pid = 0

			// Reset backoff if the process ran long enough (not a crash loop)
			if time.Since(s.startedAt) > s.backoff {
				s.backoff = 0
			}

			if ctx.Err() != nil || s.DNR {
				return
			}

			// An explicit restart means "start again now" — skip the backoff.
			if restartRequested {
				s.backoff = 0
			}

			if s.Sleep > 0 {
				if !sleepCtx(ctx, time.Duration(s.Sleep)*time.Millisecond) {
					return
				}
			}

			// Exponential backoff for repeated crashes
			if s.backoff > 0 {
				colorterm.Info(s.Name, fmt.Sprintf("restart delayed %s", s.backoff))
				if !sleepCtx(ctx, s.backoff) {
					return
				}
				s.backoff *= 2
			} else {
				s.backoff = time.Second
			}
		}
	}()

	return nil
}

// waitCmd waits for c to exit, ctx to be cancelled, or a restart to be
// requested. On cancel/restart it escalates SIGTERM -> SIGKILL with a grace
// period and always reaps the child before returning. Returns true if a
// restart was explicitly requested.
func (s *S) waitCmd(ctx context.Context, c *exec.Cmd) bool {
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()

	var restart bool
	select {
	case err := <-done:
		s.logWaitError(err)
		return false
	case <-ctx.Done():
	case <-s.restartCh:
		restart = true
	}

	// Graceful termination — signal the entire process group so that
	// grandchildren (e.g. the actual server spawned by `go run`) also
	// receive the signal and release their sockets.
	_ = s.signalGroup(syscall.SIGTERM)
	select {
	case err := <-done:
		s.logWaitError(err)
		return restart
	case <-time.After(gracePeriod):
	}

	// Force kill the entire process group.
	colorterm.Warning(s.Name, "process did not exit after SIGTERM, sending SIGKILL")
	_ = s.signalGroup(syscall.SIGKILL)
	select {
	case err := <-done:
		s.logWaitError(err)
	case <-time.After(gracePeriod):
		colorterm.Error(s.Name, "process did not exit after SIGKILL; abandoning")
	}
	return restart
}

func (s *S) logWaitError(err error) {
	if err == nil {
		colorterm.Info(s.Name, "ended")
		return
	}
	if err.Error() == "signal: killed" || err.Error() == "signal: terminated" || err.Error() == "wait: no child processes" {
		colorterm.Info(s.Name, "ended")
		return
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		colorterm.Error(s.Name, "ended with error:", err)
		return
	}
	colorterm.Error(s.Name, "error waiting for process:", err)
}

func (s *S) run(ctx context.Context, cmd string) error {
	if cmd == "" {
		return nil
	}

	c, closeOutputs := s.parse(ctx, cmd)
	defer closeOutputs()

	return c.Run()
}

// sleepCtx sleeps for d or until ctx is cancelled. Returns false if ctx was
// cancelled (caller should stop), true otherwise.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *S) resolveWriter(output string, fallback *os.File) (io.Writer, func(), error) {
	if output == "os" {
		return fallback, nil, nil
	}
	if strings.HasPrefix(output, "file:") {
		path := strings.TrimPrefix(output, "file:")
		path = strings.ReplaceAll(path, "{service-name}", s.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, fmt.Errorf("create output dir: %w", err)
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("open output file: %w", err)
		}
		return f, func() { f.Close() }, nil
	}
	return nil, nil, nil
}

// parse builds an exec.Cmd bound to ctx. The returned function closes any
// output files opened for redirection and is idempotent.
func (s *S) parse(ctx context.Context, cmd string) (*exec.Cmd, func()) {
	parts := strings.Split(cmd, " ")
	name := parts[0]
	args := parts[1:]

	c := exec.CommandContext(ctx, name, args...)
	c.Dir = coalesce.String(s.Dir, ".")
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	c.WaitDelay = gracePeriod

	var closers []func()

	if w, closer, err := s.resolveWriter(s.Output.Stdout, os.Stdout); err != nil {
		colorterm.Error(s.Name, "stdout:", err)
	} else {
		c.Stdout = w
		if closer != nil {
			closers = append(closers, closer)
		}
	}

	if w, closer, err := s.resolveWriter(s.Output.Stderr, os.Stderr); err != nil {
		colorterm.Error(s.Name, "stderr:", err)
	} else {
		c.Stderr = w
		if closer != nil {
			closers = append(closers, closer)
		}
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

	var once sync.Once
	closeOutputs := func() {
		once.Do(func() {
			for _, close := range closers {
				close()
			}
		})
	}

	return c, closeOutputs
}

// signalGroup sends a signal to the entire process group of the running child.
// This ensures grandchildren (e.g. a server spawned by `go run`) also receive
// the signal and release their resources (sockets, files, etc.).
func (s *S) signalGroup(sig syscall.Signal) error {
	if s.pid <= 0 {
		return nil
	}
	return syscall.Kill(-s.pid, sig)
}

// waitForExit polls until the process group is gone or a 5s deadline is hit.
// After waitCmd has sent SIGTERM/SIGKILL to the group and c.Wait() has reaped
// the direct child, resources (sockets, files) are already released. This poll
// is a safety net; zombies may linger but don't hold resources.
func (s *S) waitForExit(ctx context.Context) {
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if s.pid <= 0 || syscall.Kill(-s.pid, 0) != nil {
			return
		}
		select {
		case <-ctx.Done():
			_ = s.signalGroup(syscall.SIGKILL)
			return
		case <-deadline:
			return
		case <-ticker.C:
		}
	}
}
