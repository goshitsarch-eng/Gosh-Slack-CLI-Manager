package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"
)

func tempPathFor(path string) (string, error) {
	parent := filepath.Dir(path)
	name := filepath.Base(path)
	if path == "" || name == "/" || name == "." || name == ".." {
		return "", FileOperationError(fmt.Sprintf("%s has no valid file name", path))
	}
	return filepath.Join(parent, fmt.Sprintf(".%s.tmp.%d", name, time.Now().UnixNano())), nil
}

// AtomicWrite replaces path with contents via a temp file in the same
// directory, fsync and rename, keeping the original file's permissions.
func AtomicWrite(path, contents string) error {
	tempPath, err := tempPathFor(path)
	if err != nil {
		return err
	}

	tempFile, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return IOError(err)
	}
	defer tempFile.Close()

	if info, err := os.Stat(path); err == nil {
		if err := tempFile.Chmod(info.Mode().Perm()); err != nil {
			return IOError(err)
		}
	}

	if _, err := tempFile.WriteString(contents); err != nil {
		return IOError(err)
	}
	if err := tempFile.Sync(); err != nil {
		return IOError(err)
	}
	if err := tempFile.Close(); err != nil {
		return IOError(err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return IOError(err)
	}

	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		dir.Close()
	}
	return nil
}

// ErrInvalidUTF8 matches the error Rust's read_to_string reports.
var ErrInvalidUTF8 = errors.New("stream did not contain valid UTF-8")

// ReadFileString reads a whole file as UTF-8 text, failing on invalid UTF-8
// like Rust's fs::read_to_string.
func ReadFileString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", ErrInvalidUTF8
	}
	return string(data), nil
}
