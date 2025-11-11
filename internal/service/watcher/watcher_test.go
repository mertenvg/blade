package watcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoreList_Basics(t *testing.T) {
	cases := []struct{
		patterns []string
		path string
		want bool
	}{
		{[]string{"**/*.txt"}, "a/b/c/file.txt", true},
		{[]string{"**/*.txt"}, "a/b/c/file.md", false},
		{[]string{"*.md"}, "README.md", true},
		{[]string{"*.md"}, "docs/readme.md", false}, // pattern anchors to entire path
		{[]string{"docs/**"}, "docs/readme.md", true},
		{[]string{"docs/*"}, "docs/readme.md", true},
		{[]string{"docs/*"}, "docs/sub/readme.md", false},
	}
	for i, tc := range cases {
		il := NewIgnoreList(tc.patterns)
		if got := il.ShouldIgnore(tc.path); got != tc.want {
			t.Fatalf("case %d: patterns=%v path=%s got %v want %v", i, tc.patterns, tc.path, got, tc.want)
		}
	}
}

func TestFSWatcher_ScanHasChangedReset_FileLifecycle(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "x.txt")

	fs := &FSWatcher{path: file, ignore: NewIgnoreList(nil)}

	// Initial scan without file
	fs.Scan()
	if fs.HasChanged() {
		t.Errorf("unexpected change when file does not exist")
	}
	fs.Reset()

	// Create file
	if err := os.WriteFile(file, []byte("a"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	fs.Scan()
	if !fs.HasChanged() {
		t.Fatalf("expected change after file creation")
	}
	fs.Reset()

	// Modify file
	if err := os.WriteFile(file, []byte("ab"), 0o644); err != nil {
		t.Fatalf("write2: %v", err)
	}
	fs.Scan()
	if !fs.HasChanged() {
		t.Fatalf("expected change after file modification")
	}
	fs.Reset()

	// No-op scan should not report change
	fs.Scan()
	if fs.HasChanged() {
		t.Fatalf("expected no change with identical stat")
	}
	fs.Reset()
}

func TestFSWatcher_Scan_DirectoryChildren(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fs := &FSWatcher{path: dir, ignore: NewIgnoreList(nil)}

	// initial
	fs.Scan()
	if fs.HasChanged() {
		t.Errorf("unexpected change on empty dir")
	}
	fs.Reset()

	// add a child
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	fs.Scan()
	if !fs.HasChanged() {
		t.Fatalf("expected change after adding a child")
	}
	fs.Reset()

	// rescanning without changes
	fs.Scan()
	if fs.HasChanged() {
		t.Fatalf("expected no change after reset when no FS changes")
	}
}
