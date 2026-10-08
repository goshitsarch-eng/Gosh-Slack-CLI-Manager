// Package slackware detects the installed release and wraps Slackware
// configuration files and command-line tools.
package slackware

import (
	"fmt"
	"os"
	"strings"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/utils"
)

// VersionKind enumerates the known Slackware releases.
type VersionKind uint8

const (
	Current VersionKind = iota
	V15_0
	V14_2
	V14_1
	Unknown
)

// Version is a detected Slackware release. Raw holds the normalized version
// string for Unknown releases.
type Version struct {
	Kind VersionKind
	Raw  string
}

// VersionFromString parses the contents of /etc/slackware-version.
func VersionFromString(version string) Version {
	version = strings.ToLower(strings.TrimSpace(version))
	switch {
	case strings.Contains(version, "current"):
		return Version{Kind: Current}
	case strings.Contains(version, "15.0"):
		return Version{Kind: V15_0}
	case strings.Contains(version, "14.2"):
		return Version{Kind: V14_2}
	case strings.Contains(version, "14.1"):
		return Version{Kind: V14_1}
	default:
		return Version{Kind: Unknown, Raw: version}
	}
}

// MirrorPath returns the mirror directory suffix for this release.
func (v Version) MirrorPath() string {
	switch v.Kind {
	case V15_0:
		return "slackware64-15.0"
	case V14_2:
		return "slackware64-14.2"
	case V14_1:
		return "slackware64-14.1"
	default:
		return "slackware64-current"
	}
}

// DisplayName returns a human readable release name.
func (v Version) DisplayName() string {
	switch v.Kind {
	case Current:
		return "Slackware64 Current"
	case V15_0:
		return "Slackware64 15.0"
	case V14_2:
		return "Slackware64 14.2"
	case V14_1:
		return "Slackware64 14.1"
	default:
		return fmt.Sprintf("Slackware (%s)", v.Raw)
	}
}

func (v Version) String() string { return v.DisplayName() }

// DetectVersion reads /etc/slackware-version.
func DetectVersion() (Version, error) {
	const versionFile = "/etc/slackware-version"
	if _, err := os.Stat(versionFile); err != nil {
		return Version{}, utils.VersionDetectionError(
			"File /etc/slackware-version not found. Is this a Slackware system?")
	}
	content, err := utils.ReadFileString(versionFile)
	if err != nil {
		return Version{}, utils.VersionDetectionError(
			fmt.Sprintf("Failed to read /etc/slackware-version: %s", utils.IOErrorString(err)))
	}
	return VersionFromString(content), nil
}
