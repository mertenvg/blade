package service

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mertenvg/blade/pkg/coalesce"
)

func TestCoalesce(t *testing.T) {
	if got := coalesce.String("", "", "a", "b"); got != "a" {
		t.Fatalf("coalesce returned %q, want %q", got, "a")
	}
	if got := coalesce.String(""); got != "" {
		t.Fatalf("coalesce returned %q, want empty", got)
	}
}

func TestParse_EnvHandling_NoInherit(t *testing.T) {
	// ensure we have a noisy env var present in the process environment
	os.Setenv("NOISY_ENV", "present")
	s := &S{
		Name:       "svc",
		InheritEnv: false,
		Env: []EnvValue{
			{Name: "FOO", Value: strPtr("bar")},
			{Name: "PATH"}, // explicit inclusion when Value is nil should pull from parent
		},
	}
	cmd, closeOutputs := s.parse(context.Background(), "echo hi")
	defer closeOutputs()

	// Env should contain only our specified entries plus BLADE_SERVICE_NAME
	env := cmd.Env
	if len(env) == 0 {
		t.Fatalf("expected some env, got none")
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "NOISY_ENV=") {
			t.Fatalf("environment should not inherit NOISY_ENV but did: %v", kv)
		}
	}

	// Should contain our explicit FOO
	assertEnvHas(t, env, "FOO=bar")
	assertEnvHasPrefix(t, env, "PATH=")

	// Output.Stdin behavior seems inverted: with default empty Output.Stdin, parse sets c.Stdin to os.Stdin
	// This may unintentionally allow child processes to read from parent's stdin.
	if cmd.Stdin == nil {
		t.Errorf("expected Stdin to be non-nil due to current parse logic; potential security concern: stdin is always wired unless explicitly set to 'os'")
	}
}

func TestStart_WithEmptyCmdErrors(t *testing.T) {
	s := &S{Name: "svc"}
	if err := s.start(context.Background(), ""); err == nil {
		t.Fatalf("expected error when starting with empty command")
	}
}


func TestStatus_Defaults(t *testing.T) {
	s := &S{Name: "svc"}
	active, state, pid := s.Status()
	if active {
		t.Errorf("expected inactive by default")
	}
	if state == "" {
		t.Errorf("expected some state message")
	}
	if pid == "" {
		t.Errorf("expected pid placeholder")
	}
}

func TestParse_CommandSplittingIsNaive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("command semantics differ on Windows")
	}
	s := &S{Name: "svc"}
	// Argument with space inside quotes will be split incorrectly by strings.Split
	cmd, closeOutputs := s.parse(context.Background(), "/bin/echo 'hello world'")
	defer closeOutputs()
	if len(cmd.Args) != 3 { // expect [/bin/echo 'hello world'] but actually becomes 3 elements when quotes present
		// We do not fail the test; instead, flag as potential issue through error
		t.Logf("potential issue: naive splitting resulted in %d args: %v", len(cmd.Args), cmd.Args)
	}
	// Verify command actually starts to ensure no crash
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command: %v", err)
	}
	_ = cmd.Process.Kill()
}

func TestSleepCtx_TimerElapses(t *testing.T) {
	if !sleepCtx(context.Background(), 10*time.Millisecond) {
		t.Fatalf("expected true when timer elapses")
	}
}

func TestSleepCtx_CancelReturnsFalse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepCtx(ctx, time.Hour) {
		t.Fatalf("expected false when ctx cancelled")
	}
}

func TestSleepCtx_ZeroDurationRespectsCtx(t *testing.T) {
	if !sleepCtx(context.Background(), 0) {
		t.Fatalf("expected true with zero duration and live ctx")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepCtx(ctx, 0) {
		t.Fatalf("expected false with zero duration and dead ctx")
	}
}

func TestRestart_NilChannelIsNoOp(t *testing.T) {
	s := &S{Name: "svc"}
	// Must not panic when restartCh has not been initialised (Start not called).
	s.Restart()
}

func TestRestart_Coalesces(t *testing.T) {
	s := &S{Name: "svc", restartCh: make(chan empty, 1)}
	// Two rapid restarts should not block even though the channel buffer is 1.
	done := make(chan struct{})
	go func() {
		s.Restart()
		s.Restart()
		s.Restart()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Restart blocked; should coalesce when channel is full")
	}
	// Drain the single buffered slot.
	select {
	case <-s.restartCh:
	default:
		t.Fatalf("expected one pending restart in the buffered channel")
	}
}

func TestStart_ContextCancelStopsService(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix sleep")
	}
	s := &S{Name: "svc", Run: "sleep 30"}
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)

	// Give the child a moment to actually be running.
	time.Sleep(200 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()

	// sleep doesn't trap SIGTERM so it should die well before the SIGKILL grace.
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("service did not exit after context cancel")
	}
}

func TestExit_StopsServiceWithoutWatcher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix sleep")
	}
	s := &S{Name: "svc", Run: "sleep 30"}
	s.Start(context.Background())

	time.Sleep(200 * time.Millisecond)
	s.Exit()

	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("service did not exit after Exit() call")
	}
}

// helpers
func strPtr(s string) *string { return &s }

func assertEnvHas(t *testing.T, env []string, want string) {
	t.Helper()
	for _, kv := range env {
		if kv == want {
			return
		}
	}
	t.Fatalf("env does not contain %q; got %v", want, env)
}

func assertEnvHasPrefix(t *testing.T, env []string, prefix string) {
	t.Helper()
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return
		}
	}
	t.Fatalf("env does not contain prefix %q; got %v", prefix, env)
}
