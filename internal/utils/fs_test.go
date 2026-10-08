package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteReplacesContentAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, "new\n"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("got %q", data)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

func TestIOErrorStringMatchesRustFormatting(t *testing.T) {
	_, err := os.ReadFile("/nonexistent/slackware-cli-manager")
	if got := IOErrorString(err); got != "No such file or directory (os error 2)" {
		t.Fatalf("got %q", got)
	}
	if got := IOErrorString(errors.New("plain")); got != "plain" {
		t.Fatalf("got %q", got)
	}
}

func TestIOErrorMatchesAppErrorIoDisplay(t *testing.T) {
	_, err := os.ReadFile("/nonexistent/slackware-cli-manager")
	if got := IOError(err).Error(); got != "IO error: No such file or directory (os error 2)" {
		t.Fatalf("got %q", got)
	}
	if got := PlainIOError(err).Error(); got != "No such file or directory (os error 2)" {
		t.Fatalf("got %q", got)
	}
}

func TestReadFileStringRejectsInvalidUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad")
	if err := os.WriteFile(path, []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFileString(path); IOErrorString(err) != "stream did not contain valid UTF-8" {
		t.Fatalf("got %v", err)
	}
}
