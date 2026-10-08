package slackware

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

// Bootloader is the detected bootloader.
type Bootloader uint8

const (
	BootloaderLilo Bootloader = iota
	BootloaderGrub
	BootloaderUnknown
)

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DetectBootloader checks for LILO first (the Slackware default), then GRUB.
func DetectBootloader() Bootloader {
	if exists("/etc/lilo.conf") {
		return BootloaderLilo
	}
	if exists("/boot/grub/grub.cfg") || exists("/etc/default/grub") {
		return BootloaderGrub
	}
	return BootloaderUnknown
}

// Name returns the bootloader display name.
func (b Bootloader) Name() string {
	switch b {
	case BootloaderLilo:
		return "LILO"
	case BootloaderGrub:
		return "GRUB"
	default:
		return "Unknown"
	}
}

// MirrorEntry is a mirror line from /etc/slackpkg/mirrors.
type MirrorEntry struct {
	URL      string
	IsActive bool
	Region   string
}

const mirrorsPath = "/etc/slackpkg/mirrors"

// ParseMirrors reads mirrors, keeping only URLs containing versionFilter
// (when non-empty).
func ParseMirrors(versionFilter string) ([]MirrorEntry, error) {
	if !exists(mirrorsPath) {
		return nil, utils.FileOperationError("Mirrors file not found at /etc/slackpkg/mirrors")
	}
	content, err := utils.ReadFileString(mirrorsPath)
	if err != nil {
		return nil, utils.IOError(err)
	}
	return ParseMirrorsFromContent(content, versionFilter), nil
}

var regionRe = regexp.MustCompile(`mirrors\.(\w+)\.|\.(\w{2})/|/(\w{2})/`)

func extractRegion(url string) string {
	if m := regionRe.FindStringSubmatch(url); m != nil {
		for _, group := range m[1:] {
			if group != "" {
				return strings.ToUpper(group)
			}
		}
	}
	if strings.Contains(url, "kernel.org") || strings.Contains(url, "osuosl") {
		return "US"
	}
	if strings.Contains(url, "ukfast") {
		return "UK"
	}
	return "Unknown"
}

// SetActiveMirror makes mirrorURL the only active mirror, keeping a .bak copy.
func SetActiveMirror(mirrorURL string) error {
	if !exists(mirrorsPath) {
		return utils.FileOperationError("Mirrors file not found")
	}
	content, err := utils.ReadFileString(mirrorsPath)
	if err != nil {
		return utils.IOError(err)
	}
	newContent, err := RewriteMirrorsContent(content, mirrorURL)
	if err != nil {
		return err
	}
	if data, err := os.ReadFile(mirrorsPath); err == nil {
		mode := os.FileMode(0o644)
		if info, err := os.Stat(mirrorsPath); err == nil {
			mode = info.Mode().Perm()
		}
		_ = os.WriteFile(mirrorsPath+".bak", data, mode)
	}
	return utils.AtomicWrite(mirrorsPath, newContent)
}

var initdefaultRe = regexp.MustCompile(`id:\d:initdefault:`)

// SetDefaultRunlevel rewrites the initdefault entry in /etc/inittab.
func SetDefaultRunlevel(runlevel int) error {
	const inittab = "/etc/inittab"
	if !exists(inittab) {
		return utils.FileOperationError("/etc/inittab not found")
	}
	content, err := utils.ReadFileString(inittab)
	if err != nil {
		return utils.IOError(err)
	}
	text := content
	if loc := initdefaultRe.FindStringIndex(text); loc != nil {
		text = text[:loc[0]] + fmt.Sprintf("id:%d:initdefault:", runlevel) + text[loc[1]:]
	}
	return utils.AtomicWrite(inittab, text)
}

func isMirrorURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "ftp://")
}

// ParseMirrorsFromContent parses mirrors file content.
func ParseMirrorsFromContent(content, versionFilter string) []MirrorEntry {
	var mirrors []MirrorEntry
	for _, line := range rustLines(content) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var active bool
		var url string
		if strings.HasPrefix(trimmed, "#") {
			part := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if !isMirrorURL(part) {
				continue
			}
			url = part
		} else if isMirrorURL(trimmed) {
			active, url = true, trimmed
		} else {
			continue
		}
		if versionFilter != "" && !strings.Contains(url, versionFilter) {
			continue
		}
		mirrors = append(mirrors, MirrorEntry{Region: extractRegion(url), URL: url, IsActive: active})
	}
	return mirrors
}

// RewriteMirrorsContent comments out every mirror except mirrorURL.
func RewriteMirrorsContent(content, mirrorURL string) (string, error) {
	lines := rustLines(content)
	found := false
	for _, line := range lines {
		if strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#")) == mirrorURL {
			found = true
			break
		}
	}
	if !found {
		return "", utils.ConfigError(fmt.Sprintf("Mirror '%s' was not found in mirrors file", mirrorURL))
	}

	newLines := make([]string, 0, len(lines))
	active := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || (strings.HasPrefix(trimmed, "#") && !strings.Contains(trimmed, "://")) {
			newLines = append(newLines, line)
			continue
		}
		url := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		switch {
		case url == mirrorURL:
			newLines = append(newLines, url)
			active++
		case strings.HasPrefix(trimmed, "#"):
			newLines = append(newLines, line)
		default:
			newLines = append(newLines, "# "+trimmed)
		}
	}
	if active != 1 {
		return "", utils.ConfigError(fmt.Sprintf("Mirror rewrite left %d active mirrors; expected exactly one", active))
	}
	return strings.Join(newLines, "\n") + "\n", nil
}
