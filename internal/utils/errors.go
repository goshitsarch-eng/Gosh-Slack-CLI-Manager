// Package utils holds small filesystem and error helpers shared by the app.
package utils

import (
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"syscall"
	"unicode"
	"unicode/utf8"
)

// AppError mirrors the error categories of the original implementation so
// user-facing messages keep the same wording.
type AppError struct {
	msg string
}

func (e *AppError) Error() string { return e.msg }

// VersionDetectionError reports a failure to detect the Slackware version.
func VersionDetectionError(detail string) error {
	return &AppError{msg: "Failed to detect Slackware version: " + detail}
}

// FileOperationError reports a failed file operation.
func FileOperationError(detail string) error {
	return &AppError{msg: "File operation failed: " + detail}
}

// ConfigError reports a configuration problem.
func ConfigError(detail string) error {
	return &AppError{msg: "Configuration error: " + detail}
}

// IOError wraps an OS error so it renders like a libc/strerror message with
// its errno, e.g. "No such file or directory (os error 2)".
func IOError(err error) error {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return err
	}
	return &AppError{msg: IOErrorString(err)}
}

// IOErrorString formats err the way the original tool displayed I/O errors.
func IOErrorString(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, exec.ErrNotFound) {
		return errnoString(syscall.ENOENT)
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errnoString(errno)
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return IOErrorString(pathErr.Err)
	}
	return err.Error()
}

func errnoString(errno syscall.Errno) string {
	text := errno.Error()
	if r, size := utf8.DecodeRuneInString(text); size > 0 {
		text = string(unicode.ToUpper(r)) + text[size:]
	}
	return fmt.Sprintf("%s (os error %d)", text, int(errno))
}
