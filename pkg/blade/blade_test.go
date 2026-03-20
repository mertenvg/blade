package blade_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func setupTempModuleOrSkip(t *testing.T, dir string) {
	t.Helper()
	cwd, _ := os.Getwd()
	gomod := "module tmpmod\n\nrequire github.com/mertenvg/blade v0.0.0\n\nreplace github.com/mertenvg/blade => " + filepath.Clean(filepath.Join(cwd, "..", "..")) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	cmd := exec.Command("go", "mod", "download")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("skipping: unable to download module deps in temp module: %v; out=%s", err, string(out))
	}
}

const helperProg = `package main
import (
    blade "github.com/mertenvg/blade/pkg/blade"
)
func main(){
    blade.Done()
}
`

const helperNoDone = `package main
import (
    "time"
    _ "github.com/mertenvg/blade/pkg/blade"
)
func main(){
    time.Sleep(1*time.Second)
}
`

func TestPIDFile_CreatedAndRemoved(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	tmp := t.TempDir()
	prog := filepath.Join(tmp, "main.go")
	if err := os.WriteFile(prog, []byte(helperProg), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	// create a temp go.mod that replaces module to current repo root and download deps
	setupTempModuleOrSkip(t, tmp)
	service := "svc_test"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "BLADE_SERVICE_NAME="+service)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go run failed: %v; out=%s", err, string(out))
	}
	pidFile := filepath.Join(tmp, "."+service+".pid")
	if _, err := os.Stat(pidFile); err == nil {
		t.Fatalf("pid file should have been removed by Done(), but exists: %s", pidFile)
	}
}

func TestPIDFile_SymlinkOverwriteRisk(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.txt")
	if err := os.WriteFile(target, []byte("SECRET"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	service := "svc_link"
	pidLink := filepath.Join(tmp, "."+service+".pid")
	// create symlink from pid file path to our target file
	if err := os.Symlink(target, pidLink); err != nil {
		t.Skipf("cannot create symlink on this system: %v", err)
	}

	// program that imports blade but does not call Done()
	prog := filepath.Join(tmp, "main_nodone.go")
	if err := os.WriteFile(prog, []byte(helperNoDone), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	setupTempModuleOrSkip(t, tmp)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Dir = tmp
	cmd.Env = append(append([]string{}, os.Environ()...), "BLADE_SERVICE_NAME="+service)
	// start process and wait a bit to allow init() to run and write to pid file (following symlink)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start go run: %v", err)
	}
	// poll for overwrite up to 2 seconds
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(target)
		if err != nil {
			break
		}
		if string(b) != "SECRET" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	b, err := os.ReadFile(target)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("read target: %v", err)
	}
	content := string(b)
	if content == "SECRET" {
		_ = cmd.Process.Kill()
		t.Skipf("could not observe overwrite via symlink (content unchanged). On systems with symlink protections this may be blocked; risk remains if not mitigated.")
	}
	// cleanup
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

// helperDoneOnly calls blade.Done() immediately. The init() will also run,
// writing the process's own PID. We pre-seed the file with a foreign PID
// before launching this helper to verify Done() leaves the file intact.
const helperDoneOnly = `package main
import blade "github.com/mertenvg/blade/pkg/blade"
func main(){ blade.Done() }
`

func TestDone_DoesNotRemoveOtherPID(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	tmp := t.TempDir()
	prog := filepath.Join(tmp, "main.go")
	if err := os.WriteFile(prog, []byte(helperDoneOnly), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	setupTempModuleOrSkip(t, tmp)

	service := "svc_other"
	pidFile := filepath.Join(tmp, "."+service+".pid")
	foreignPID := "999999"

	// Strategy: launch a helper that sleeps between init() and Done().
	// During the sleep, overwrite the PID file with a foreign PID.
	// When Done() runs, it should see the foreign PID and leave the file.
	helperWithSleep := `package main
import (
    "time"
    blade "github.com/mertenvg/blade/pkg/blade"
)
func main(){
    time.Sleep(2*time.Second)
    blade.Done()
}
`
	if err := os.WriteFile(prog, []byte(helperWithSleep), 0o644); err != nil {
		t.Fatalf("rewrite helper: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()

	cmd := exec.CommandContext(ctx2, "go", "run", ".")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "BLADE_SERVICE_NAME="+service)

	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	// Wait for init() to write the PID file with the process's own PID
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(pidFile); err == nil && string(b) != foreignPID {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Now overwrite the PID file with a foreign PID (simulating a newer process taking over)
	if err := os.WriteFile(pidFile, []byte(foreignPID), 0o644); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("overwrite pid file: %v", err)
	}

	// Wait for the helper to finish (Done() will run)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper failed: %v", err)
	}

	// The PID file should still exist with the foreign PID
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("pid file was removed by Done() but should have been preserved (foreign PID): %v", err)
	}
	if string(b) != foreignPID {
		t.Fatalf("pid file content changed: got %q, want %q", string(b), foreignPID)
	}
}
