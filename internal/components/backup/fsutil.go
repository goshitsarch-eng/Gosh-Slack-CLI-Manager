package backup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
	"unicode"
)

// trimStartMatches removes every leading repetition of prefix, like Rust's
// str::trim_start_matches.
func trimStartMatches(s, prefix string) string {
	if prefix == "" {
		return s
	}
	for strings.HasPrefix(s, prefix) {
		s = s[len(prefix):]
	}
	return s
}

// copyFile mirrors Rust's fs::copy on Linux: the source must be a regular
// file (after following symlinks), the destination is created/truncated with
// the source's permission bits, and those bits are then applied to it.
func copyFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("the source path is neither a regular file nor a symlink to a regular file")
	}
	perm := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	dst, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer dst.Close()
	dstInfo, err := dst.Stat()
	if err != nil {
		return err
	}
	if dstInfo.Mode().IsRegular() {
		if err := dst.Chmod(perm); err != nil {
			return err
		}
	}
	_, err = io.Copy(dst, src)
	return err
}

// removeDirAll mirrors Rust's fs::remove_dir_all: a missing path is an
// error, a symlink is unlinked, and a non-directory fails with ENOTDIR.
func removeDirAll(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(path)
	}
	if !info.IsDir() {
		return syscall.ENOTDIR
	}
	return os.RemoveAll(path)
}

// stamp is a backup timestamp with chrono's leap-second representation
// (second 59 plus a leap flag displayed as second 60).
type stamp struct {
	t    time.Time
	leap bool
}

func nowStamp() stamp { return stamp{t: time.Now()} }

func (s stamp) less(o stamp) bool {
	if s.t.Equal(o.t) {
		return !s.leap && o.leap
	}
	return s.t.Before(o.t)
}

// format renders the stamp like chrono's "%Y-%m-%d %H:%M:%S".
func (s stamp) format() string {
	t := s.t
	year := t.Year()
	var y string
	switch {
	case year >= 0 && year <= 9999:
		y = fmt.Sprintf("%04d", year)
	default:
		y = fmt.Sprintf("%+05d", year)
	}
	sec := t.Second()
	if s.leap {
		sec = 60
	}
	return fmt.Sprintf("%s-%02d-%02d %02d:%02d:%02d", y, int(t.Month()), t.Day(), t.Hour(), t.Minute(), sec)
}

// scanNumber mirrors chrono's scan::number: between min and max ASCII digits.
func scanNumber(s string, minDigits, maxDigits int) (string, int64, bool) {
	if len(s) < minDigits {
		return s, 0, false
	}
	var n int64
	for i := 0; i < len(s) && i < maxDigits; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			if i < minDigits {
				return s, 0, false
			}
			return s[i:], n, true
		}
		if n > (1<<63-1-int64(c-'0'))/10 {
			return s, 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return s[min(maxDigits, len(s)):], n, true
}

// parseBackupStamp parses s with chrono's NaiveDateTime::parse_from_str
// semantics for "%Y%m%d_%H%M%S", then (as the original does) treats the
// naive value as UTC and attaches the current local offset.
func parseBackupStamp(s string) (stamp, bool) {
	var vals [6]int64
	for i := 0; i < 6; i++ {
		if i == 3 {
			if !strings.HasPrefix(s, "_") {
				return stamp{}, false
			}
			s = s[1:]
		}
		s = strings.TrimLeftFunc(s, unicode.IsSpace)
		var v int64
		var ok bool
		if i == 0 {
			switch {
			case strings.HasPrefix(s, "-"):
				s, v, ok = scanNumber(s[1:], 1, int(^uint(0)>>1))
				v = -v
			case strings.HasPrefix(s, "+"):
				s, v, ok = scanNumber(s[1:], 1, int(^uint(0)>>1))
			default:
				s, v, ok = scanNumber(s, 1, 4)
			}
		} else {
			s, v, ok = scanNumber(s, 1, 2)
		}
		if !ok {
			return stamp{}, false
		}
		vals[i] = v
	}
	if s != "" {
		return stamp{}, false
	}
	year, month, day, hour, minute, second := vals[0], vals[1], vals[2], vals[3], vals[4], vals[5]
	const minYear, maxYear = -262144, 262143
	if year < minYear || year > maxYear || month < 1 || month > 12 || day < 1 || day > 31 ||
		hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 60 {
		return stamp{}, false
	}
	// Reject impossible dates (e.g. February 30th).
	probe := time.Date(int(year), time.Month(month), int(day), 0, 0, 0, 0, time.UTC)
	if probe.Day() != int(day) || probe.Month() != time.Month(month) {
		return stamp{}, false
	}
	leap := second == 60
	if leap {
		second = 59
	}
	_, offset := time.Now().Zone()
	zone := time.FixedZone("", offset)
	t := time.Date(int(year), time.Month(month), int(day), int(hour), int(minute), int(second), 0, time.UTC).In(zone)
	return stamp{t: t, leap: leap}, true
}
