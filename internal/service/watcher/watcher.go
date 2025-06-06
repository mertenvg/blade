package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mertenvg/blade/pkg/colorterm"
)

type IgnoreList struct {
	regexes []*regexp.Regexp
}

func (il *IgnoreList) ShouldIgnore(path string) bool {
	for _, regex := range il.regexes {
		if regex.MatchString(path) {
			return true
		}
	}
	return false
}

func NewIgnoreList(paths []string) *IgnoreList {
	regexes := make([]*regexp.Regexp, len(paths))
	for i, p := range paths {
		p = regexp.QuoteMeta(p)
		p = strings.ReplaceAll(p, `\*\*`, `[\w\-. \/\\]+`)
		p = strings.ReplaceAll(p, `\*`, `[\w\-. ]+`)
		pattern := fmt.Sprintf("^%s$", p)
		regex := regexp.MustCompile(pattern)
		regexes[i] = regex
	}
	return &IgnoreList{regexes: regexes}
}

type Watcher interface {
	Scan()
	HasChanged() bool
	Reset()
}

type Watchers []Watcher

func (ws Watchers) Scan() {
	for _, w := range ws {
		w.Scan()
	}
}

func (ws Watchers) HasChanged() bool {
	for _, w := range ws {
		if w.HasChanged() {
			return true
		}
	}
	return false
}

func (ws Watchers) Reset() {
	for _, w := range ws {
		w.Reset()
	}
}

type FSWatcherConfig struct {
	Path   *string  `yaml:"path,omitempty"`
	Paths  []string `yaml:"paths,omitempty"`
	Ignore []string `yaml:"ignore,omitempty"`
}

type W struct {
	FS *FSWatcherConfig `yaml:"fs"`

	stop context.CancelFunc
}

func (w *W) Start(action func()) {
	ctx, cancel := context.WithCancel(context.Background())
	w.stop = cancel

	var watchers Watchers

	if w.FS != nil {
		ignore := NewIgnoreList(w.FS.Ignore)
		if w.FS.Path != nil {
			watchers = append(watchers, &FSWatcher{path: *w.FS.Path, ignore: ignore})
			colorterm.Info("watching", *w.FS.Path)
		}
		for _, p := range w.FS.Paths {
			watchers = append(watchers, &FSWatcher{path: p, ignore: ignore})
			colorterm.Info("watching", p)
		}
	}

	if watchers == nil {
		return
	}

	// first run do nothing
	watchers.Scan()
	watchers.Reset()

	go func(ctx context.Context, action func()) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		timer := time.NewTimer(time.Second)
		timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				action()
			case <-ticker.C:
				watchers.Scan()
				if watchers.HasChanged() {
					timer.Reset(time.Second)
					watchers.Reset()
				}
			}
		}
	}(ctx, action)
}

func (w *W) Stop() {
	if w != nil && w.stop != nil {
		w.stop()
	}
}

func statEqual(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return (a == nil) == (b == nil)
	}
	return a.Size() == b.Size() && a.ModTime() == b.ModTime()
}

type FSWatcher struct {
	path         string
	shouldIgnore bool
	ignore       *IgnoreList
	stat         os.FileInfo
	isChanged    bool
	children     []*FSWatcher
}

func (fs *FSWatcher) Scan() {
	if fs.shouldIgnore {
		return
	}
	if fs.stat == nil && fs.ignore.ShouldIgnore(fs.path) {
		fs.shouldIgnore = true
		return
	}

	stat, err := os.Stat(fs.path)
	if err != nil {
		colorterm.Error(fs.path, err)
		return
	}

	if !stat.IsDir() {
		fs.isChanged = !statEqual(fs.stat, stat)
	}
	fs.stat = stat

	if stat.IsDir() {
		fMap := make(map[string]*FSWatcher)
		for _, w := range fs.children {
			fMap[w.path] = w
		}
		files, err := os.ReadDir(fs.path)
		if err != nil {
			colorterm.Error(fs.path, err)
			return
		}
		var children []*FSWatcher
		for _, f := range files {
			p := filepath.Join(fs.path, f.Name())
			w, ok := fMap[p]
			if !ok {
				w = &FSWatcher{path: p, ignore: fs.ignore}
				fs.isChanged = true
			}
			children = append(children, w)
			w.Scan()
		}
		if len(children) != len(fs.children) {
			fs.isChanged = true
		}
		fs.children = children
	}

	if fs.isChanged {
		colorterm.Debug(fs.path, "changed")
	}
}

func (fs *FSWatcher) HasChanged() bool {
	if fs.isChanged {
		return true
	}
	if fs.stat != nil && fs.stat.IsDir() {
		for _, f := range fs.children {
			if f.HasChanged() {
				return true
			}
		}
	}
	return false
}

func (fs *FSWatcher) Reset() {
	fs.isChanged = false
	if fs.stat != nil && fs.stat.IsDir() {
		for _, f := range fs.children {
			f.Reset()
		}
	}
}
