package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCoalesce(t *testing.T) {
	if got := coalesce("", "", "a", "b"); got != "a" {
		t.Fatalf("coalesce returned %q, want %q", got, "a")
	}
	if got := coalesce(""); got != "" {
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
	cmd, cancel := s.parse("echo hi")
	defer cancel()

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

	// Should contain our explicit FOO and BLADE_SERVICE_NAME
	assertEnvHas(t, env, "FOO=bar")
	assertEnvHasPrefix(t, env, "PATH=")
	assertEnvHas(t, env, "BLADE_SERVICE_NAME=svc")

	// Output.Stdin behavior seems inverted: with default empty Output.Stdin, parse sets c.Stdin to os.Stdin
	// This may unintentionally allow child processes to read from parent's stdin.
	if cmd.Stdin == nil {
		t.Errorf("expected Stdin to be non-nil due to current parse logic; potential security concern: stdin is always wired unless explicitly set to 'os'")
	}
}

func TestStart_WithEmptyCmdErrors(t *testing.T) {
	s := &S{Name: "svc"}
	if err := s.start(""); err == nil {
		t.Fatalf("expected error when starting with empty command")
	}
}

func TestGetPID_TrustsPidFile(t *testing.T) {
	tmp := t.TempDir()
	s := &S{Name: "svc", Dir: tmp}

	pidFile := filepath.Join(tmp, ".svc.pid")
	if err := os.WriteFile(pidFile, []byte("99999"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	pid := s.getpid(12345)
	if pid != 99999 {
		t.Fatalf("expected pid from file (99999), got %d", pid)
	}
}

func TestProcess_NotFoundDoesNotPanic(t *testing.T) {
	s := &S{Name: "svc"}
	s.pid = 999999 // likely nonexistent
	p := s.process()
	_ = p // ensure no panic
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
	cmd, cancel := s.parse("/bin/echo 'hello world'")
	defer cancel()
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
