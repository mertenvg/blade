package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/mertenvg/blade/internal/service/watcher"
)

// TestHelperPortServer is invoked as a subprocess by integration tests.
// When BLADE_TEST_HELPER=port-server it listens on the port specified by
// BLADE_TEST_PORT and waits for SIGTERM.
func TestHelperPortServer(t *testing.T) {
	if os.Getenv("BLADE_TEST_HELPER") != "port-server" {
		return
	}

	port := os.Getenv("BLADE_TEST_PORT")
	if port == "" {
		fmt.Fprintln(os.Stderr, "BLADE_TEST_PORT not set")
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "listening on port %s (pid %d)\n", port, os.Getpid())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig

	ln.Close()
	fmt.Fprintf(os.Stdout, "stopped (pid %d)\n", os.Getpid())
	os.Exit(0)
}

// TestRestart_ProcessGroupKill_ReleasesPort verifies that when a file-watcher
// triggers a restart, the entire process group is killed — including
// grandchildren that hold a TCP port — so the new instance can bind cleanly.
//
// The service command uses a shell wrapper that backgrounds the actual server
// process, simulating the `go run ./cmd/server` pattern where the direct child
// (go / sh) is not the process that holds the port.
func TestRestart_ProcessGroupKill_ReleasesPort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only test")
	}

	testBin, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	portStr := strconv.Itoa(port)

	// Create watched directory with a trigger file.
	watchDir := t.TempDir()
	triggerFile := filepath.Join(watchDir, "trigger.txt")
	if err := os.WriteFile(triggerFile, []byte("v0"), 0644); err != nil {
		t.Fatal(err)
	}

	// Log directory — inspectable after the test.
	logDir := t.TempDir()
	t.Logf("log dir: %s", logDir)

	// Wrapper script that spawns the helper as a background child and waits.
	// This creates a two-level process tree:
	//   sh (PGID leader, direct child of blade)
	//     └─ test-binary (grandchild, holds the TCP port)
	// Without process-group signalling, SIGTERM to sh leaves the grandchild
	// running and the port remains bound.
	wrapperDir := t.TempDir()
	wrapperScript := filepath.Join(wrapperDir, "wrapper.sh")
	if err := os.WriteFile(wrapperScript, []byte(fmt.Sprintf(
		"#!/bin/sh\n%s -test.run=^TestHelperPortServer$ -test.v &\nwait\n",
		testBin,
	)), 0755); err != nil {
		t.Fatal(err)
	}

	serviceDir := t.TempDir()
	watchPath := watchDir

	s := &S{
		Name:       "port-test",
		Run:        "sh " + wrapperScript,
		Dir:        serviceDir,
		InheritEnv: true,
		Env: []EnvValue{
			{Name: "BLADE_TEST_HELPER", Value: strPtr("port-server")},
			{Name: "BLADE_TEST_PORT", Value: strPtr(portStr)},
		},
		Watch: &watcher.W{
			FS: &watcher.FSWatcherConfig{
				Path: &watchPath,
			},
		},
		Output: Output{
			Stdout: fmt.Sprintf("file:%s/stdout.log", logDir),
			Stderr: fmt.Sprintf("file:%s/stderr.log", logDir),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer func() {
		cancel()
		s.Wait()
		// Dump logs for inspection.
		stdout, _ := os.ReadFile(filepath.Join(logDir, "stdout.log"))
		stderr, _ := os.ReadFile(filepath.Join(logDir, "stderr.log"))
		t.Logf("=== stdout ===\n%s", stdout)
		t.Logf("=== stderr ===\n%s", stderr)
	}()

	s.Start(ctx)

	if !waitForPort(t, portStr, 10*time.Second) {
		t.Fatalf("service did not start listening on port %s", portStr)
	}
	t.Logf("initial: service listening on port %s", portStr)

	const restarts = 3
	for i := 1; i <= restarts; i++ {
		// Let the watcher settle before triggering.
		time.Sleep(2 * time.Second)

		if err := os.WriteFile(triggerFile, []byte(fmt.Sprintf("v%d", i)), 0644); err != nil {
			t.Fatalf("write trigger file: %v", err)
		}

		// Watcher polls every 1 s, debounces 1 s, then restart + startup.
		if !waitForPortRestart(t, portStr, 15*time.Second) {
			t.Fatalf("restart %d: service did not come back on port %s", i, portStr)
		}
		t.Logf("restart %d: service back on port %s", i, portStr)
	}
}

// waitForPort polls until a TCP connection to 127.0.0.1:port succeeds or
// timeout is reached.
func waitForPort(t *testing.T, port string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// waitForPortRestart waits for the port to go down (old process stopping) and
// then come back up (new process started). Returns false on timeout.
func waitForPortRestart(t *testing.T, port string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)

	// Phase 1: wait for port to go down.
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 100*time.Millisecond)
		if err != nil {
			break
		}
		conn.Close()
		time.Sleep(100 * time.Millisecond)
	}

	// Phase 2: wait for port to come back up.
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
