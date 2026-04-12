package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime/debug"
	"time"

	"github.com/mertenvg/blade/pkg/colorterm"
)

const modulePath = "github.com/mertenvg/blade"

// version can be set via ldflags: go build -ldflags "-X main.version=v1.0.0"
var version string

func getVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "(devel)"
}

func printVersion() {
	colorterm.Info("blade", getVersion())
}

type moduleInfo struct {
	Version string `json:"Version"`
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get("https://proxy.golang.org/" + modulePath + "/@latest")
	if err != nil {
		return "", fmt.Errorf("version: fetch latest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version: fetch latest: unexpected status %d", resp.StatusCode)
	}

	var info moduleInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("version: decode response: %w", err)
	}

	return info.Version, nil
}

func checkForUpdates() {
	current := getVersion()

	latest, err := fetchLatestVersion()
	if err != nil {
		colorterm.Error("Failed to check for updates:", err)
		os.Exit(1)
	}

	if current == latest {
		colorterm.Success("blade is up to date:", current)
		return
	}

	colorterm.Infof("current: %s, available: %s, run \"blade update\" to install %s", current, latest, latest)
}

func update() {
	latest, err := fetchLatestVersion()
	if err != nil {
		colorterm.Error("Failed to check for updates:", err)
		os.Exit(1)
	}

	current := getVersion()
	if current == latest {
		colorterm.Success("blade is already up to date:", current)
		return
	}

	colorterm.Infof("updating blade from %s to %s...", current, latest)

	cmd := exec.Command("go", "install", modulePath+"@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		colorterm.Error("Failed to update:", err)
		os.Exit(1)
	}

	colorterm.Success("blade updated to", latest)
}
