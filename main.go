package main

import (
	"bytes"
	"context"
	"github.com/mertenvg/grok"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/mertenvg/blade/internal/service"
	"github.com/mertenvg/blade/pkg/colorterm"
)

const RecursionLimit = 10

var envVarValuePlaceholder = regexp.MustCompile(`\{\$([a-zA-Z_][a-zA-Z0-9_]*)}`)

func ReadDirRecursive(dir string, depth int) []byte {
	if depth > RecursionLimit {
		colorterm.Warningf("recursion limit (%v) reached at '%s'", RecursionLimit, dir)
		return nil
	}
	var buf bytes.Buffer
	entries, err := os.ReadDir(dir)
	if err != nil {
		colorterm.Warningf("couldn't read from '%s': %w", dir, err)
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			data := ReadDirRecursive(filepath.Join(dir, e.Name()), depth+1)
			if data != nil {
				buf.Write(data)
			}
			continue
		}
		if !slices.Contains([]string{".yaml", ".yml"}, strings.ToLower(filepath.Ext(e.Name()))) {
			// not a YAML file, skipping!
			continue
		}
		buf.Write(TryFile(filepath.Join(dir, e.Name())))
	}
	return nil
}

func TryFile(path string) []byte {
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			colorterm.Warningf("Couldn't load config file '%s': %v", path, err)
			return nil
		}
		if len(data) > 0 {
			return append(data, '\n')
		}
		return nil
	}
	return nil
}

func LoadConfig() []byte {
	if data := TryFile("./blade.yaml"); data != nil {
		return data
	}
	if data := TryFile("./blade.yml"); data != nil {
		return data
	}
	if data := ReadDirRecursive("./.blade", 0); data != nil {
		return data
	}
	return nil
}

func InheritRecursive(child *service.S, lookup map[string]*service.S, depth int) {
	if depth > RecursionLimit {
		colorterm.Warningf("recursion limit (%v) reached at '%s'", RecursionLimit, child.Name)
		return
	}
	if child.From != "" {
		if parent, ok := lookup[child.From]; ok {
			if parent != child {
				InheritRecursive(parent, lookup, depth+1)
			}
			child.InheritFrom(parent)
		}
	}
}

func ResolveValueRecursive(value string, lookup map[string]string, depth int) string {
	if depth > RecursionLimit {
		colorterm.Warningf("recursion limit (%v) reached at '%s'", RecursionLimit, value)
		return value
	}
	matches := envVarValuePlaceholder.FindAllStringSubmatch(value, -1)
	if matches != nil {
		for _, m := range matches {
			v, ok := lookup[m[1]]
			if !ok {
				v = os.Getenv(m[1])
			}
			value = strings.ReplaceAll(value, m[0], ResolveValueRecursive(v, lookup, depth+1))
		}
	}
	return value
}

func main() {
	data := LoadConfig()
	if len(data) == 0 {
		colorterm.Error("Couldn't find config: expected ./blade.yaml or ./blade directory with YAML files")
		os.Exit(1)
	}

	var conf []*service.S

	if err := yaml.Unmarshal(data, &conf); err != nil {
		colorterm.Error("Couldn't parse configuration:", err)
		os.Exit(1)
	}

	services := make(map[string]*service.S)
	groups := make(map[string][]*service.S)
	for _, s := range conf {
		services[s.Name] = s
		if len(s.Tags) > 0 {
			for _, t := range s.Tags {
				groups[t] = append(groups[t], s)
			}
		}
	}

	for _, s := range conf {
		// resolve inheritance
		InheritRecursive(s, services, 0)

		// resolve env values
		env := make(map[string]string)
		for _, e := range s.Env {
			var v string
			if e.Value == nil {
				v = os.Getenv(e.Name)
			} else {
				v = *e.Value
			}
			env[e.Name] = v
		}
		for n, e := range env {
			env[n] = ResolveValueRecursive(e, env, 0)
		}
		for i, e := range s.Env {
			v := env[e.Name]
			s.Env[i].Value = &v
		}
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
			var run []*service.S
			if len(args) > 2 {
				// allow selecting by service name or group name
				added := make(map[string]struct{})
				for _, token := range args[2:] {
					if s, ok := services[token]; ok {
						if _, seen := added[s.Name]; !seen {
							run = append(run, s)
							added[s.Name] = struct{}{}
						}
						continue
					}
					if gs, ok := groups[token]; ok {
						for _, s := range gs {
							if _, seen := added[s.Name]; !seen {
								run = append(run, s)
								added[s.Name] = struct{}{}
							}
						}
						continue
					}
					colorterm.Error("Couldn't find service or group", token)
					os.Exit(1)
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
				grok.V(s)
				s.Start()
				go func(s *service.S) {
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
		if len(groups) > 0 {
			colorterm.Info("Tags available:")
			for g := range groups {
				colorterm.Info(" -", g)
			}
		}
		colorterm.None("Usage: blade run")
		colorterm.None("Or: blade run <name-or-tag> [<name-or-tag> ...]")
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
					active, state, pid := s.Status()
					if active {
						colorterm.Success(s.Name, pid, state)
					} else {
						colorterm.Error(s.Name, pid, state)
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
