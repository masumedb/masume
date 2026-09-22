package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/masumedb/masume/internal/core"
)

func TestExpandHomePathExpandsALeadingTilde(t *testing.T) {
	home := core.HomeDirectory()
	if home == "" {
		t.Skip("this process has no home directory")
	}
	if core.ExpandHomePath("~") != home {
		t.Errorf("~ expands to %q, wanted the home directory", core.ExpandHomePath("~"))
	}
	held := core.ExpandHomePath("~/masume/history")
	want := filepath.Join(home, "masume", "history")
	if held != want {
		t.Errorf("~/masume/history expands to %q, wanted %q", held, want)
	}
	if core.ExpandHomePath("/tmp/shop.db") != "/tmp/shop.db" {
		t.Error("an absolute path was rewritten")
	}
}

func TestResolveStatePathUsesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	held := core.ResolveStatePath("ai-chat.log")
	want := filepath.Join("/tmp/state", "masume", "ai-chat.log")
	if held != want {
		t.Errorf("the path reads %q, wanted %q", held, want)
	}
}

func TestResolveStatePathFallsBackBesideTheHomeDirectory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	home := core.HomeDirectory()
	if home == "" {
		t.Skip("this process has no home directory")
	}
	held := core.ResolveStatePath("history.json")
	want := filepath.Join(home, ".local", "state", "masume", "history.json")
	if held != want {
		t.Errorf("the path reads %q, wanted %q", held, want)
	}
}

func TestWriteFileWhollyLeavesNoTemporaryFileBehind(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "book.md")

	if err := core.WriteFileWholly(path, []byte("one"), 0o600); err != nil {
		t.Fatalf("the first write failed: %v", err)
	}
	if err := core.WriteFileWholly(path, []byte("two"), 0o600); err != nil {
		t.Fatalf("the second write failed: %v", err)
	}

	held, err := os.ReadFile(path)
	if err != nil || string(held) != "two" {
		t.Errorf("the file holds %q (%v), wanted the second write", held, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the directory holds %d files, wanted the written one alone", len(entries))
	}
	state, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode().Perm() != 0o600 {
		t.Errorf("the file stands at mode %v", state.Mode().Perm())
	}
}

func TestWriteFileWhollyKeepsTheFileWhereTheDirectoryIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone", "book.md")

	if err := core.WriteFileWholly(path, []byte("one"), 0o600); err == nil {
		t.Error("a write into a directory that is not there reported no error")
	}
}
