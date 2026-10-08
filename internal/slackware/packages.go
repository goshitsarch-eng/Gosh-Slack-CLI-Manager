package slackware

import "strings"

// PackageInfo describes a SlackBuilds.org package.
type PackageInfo struct {
	Name        string
	Category    string
	Description string
}

// PackageManager searches SlackBuilds.org packages.
type PackageManager struct {
	executor Executor
}

// NewPackageManager returns a package manager.
func NewPackageManager() PackageManager { return PackageManager{executor: NewExecutor()} }

// Search runs sbofind. The error string is shown to the user as-is.
func (m PackageManager) Search(query string) ([]PackageInfo, string, bool) {
	result := m.executor.Sbofind(query)
	if !result.Success {
		if result.Stderr == "" {
			return nil, "sbofind did not return any output", false
		}
		return nil, result.Stderr, false
	}
	return ParseSbofindOutput(result.Stdout), "", true
}

// ParseSbofindOutput parses sbofind output into package entries.
func ParseSbofindOutput(output string) []PackageInfo {
	var packages []PackageInfo
	var name, category, description string

	flush := func() {
		if name != "" {
			packages = append(packages, PackageInfo{Name: name, Category: category, Description: description})
			name, category, description = "", "", ""
		}
	}

	for _, raw := range rustLines(output) {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}
		// sbofind output format:
		// SBo:    category/package
		// Path:   /var/lib/sbopkg/...
		// info:   Description line
		if strings.HasPrefix(line, "SBo:") {
			path := strings.TrimSpace(trimStartMatches(line, "SBo:"))
			if idx := strings.Index(path, "/"); idx >= 0 {
				category = path[:idx]
				name = path[idx+1:]
			} else {
				name = path
			}
		} else if strings.HasPrefix(line, "info:") {
			description = strings.TrimSpace(trimStartMatches(line, "info:"))
		}
	}
	flush()
	return packages
}

func trimStartMatches(s, prefix string) string {
	for strings.HasPrefix(s, prefix) {
		s = s[len(prefix):]
	}
	return s
}

func rustLines(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	last := len(parts) - 1
	if parts[last] == "" {
		parts = parts[:last]
	}
	for i := range parts {
		// Only "\r\n" is a line ending; a final "\r" without "\n" stays.
		if i < last {
			parts[i] = strings.TrimSuffix(parts[i], "\r")
		}
	}
	return parts
}
