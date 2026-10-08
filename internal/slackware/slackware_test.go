package slackware

import (
	"strings"
	"testing"
)

func TestVersionParsing(t *testing.T) {
	cases := map[string]VersionKind{
		"Slackware 15.0":           V15_0,
		"Slackware 14.2":           V14_2,
		"Slackware Linux -current": Current,
	}
	for in, want := range cases {
		if got := VersionFromString(in).Kind; got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
	if got := VersionFromString(" Slackware 13.37\n").DisplayName(); got != "Slackware (slackware 13.37)" {
		t.Errorf("unknown display name %q", got)
	}
}

func TestMirrorPath(t *testing.T) {
	if got := (Version{Kind: V15_0}).MirrorPath(); got != "slackware64-15.0" {
		t.Error(got)
	}
	if got := (Version{Kind: Current}).MirrorPath(); got != "slackware64-current" {
		t.Error(got)
	}
}

func TestMissingCommandErrorIdentifiesProgram(t *testing.T) {
	result := NewExecutor().Execute("/nonexistent/slackware-cli-manager-test-command")
	if result.Success || result.Stdout != "" {
		t.Fatalf("unexpected result %+v", result)
	}
	want := "Unable to start /nonexistent/slackware-cli-manager-test-command: No such file or directory (os error 2)"
	if result.Stderr != want {
		t.Fatalf("got %q", result.Stderr)
	}
}

func TestExecuteStreamingCollectsBothStreams(t *testing.T) {
	var seen []string
	result := NewExecutor().ExecuteStreaming("sh", []string{"-c", "echo one; echo two >&2; echo three; exit 3"},
		func(src StreamSource, line string) {
			seen = append(seen, line)
		})
	if result.Success {
		t.Fatal("expected failure exit status")
	}
	if result.Stdout != "one\nthree" || result.Stderr != "two" {
		t.Fatalf("got %+v", result)
	}
	if len(seen) != 3 {
		t.Fatalf("streamed %v", seen)
	}
}

func TestParseSbofindOutputParsesMultipleEntries(t *testing.T) {
	output := `
SBo:    desktop/foo
Path:   /var/lib/sbopkg/SBo/15.0/desktop/foo
info:   Foo desktop utility

SBo:    system/bar
Path:   /var/lib/sbopkg/SBo/15.0/system/bar
info:   Bar system utility
`
	packages := ParseSbofindOutput(output)
	if len(packages) != 2 {
		t.Fatalf("got %d", len(packages))
	}
	if packages[0] != (PackageInfo{Name: "foo", Category: "desktop", Description: "Foo desktop utility"}) {
		t.Fatalf("got %+v", packages[0])
	}
	if packages[1].Name != "bar" || packages[1].Category != "system" {
		t.Fatalf("got %+v", packages[1])
	}
}

func TestParseSbofindOutputKeepsLastEntryWithoutTrailingBlankLine(t *testing.T) {
	output := `
SBo:    network/curlie
Path:   /var/lib/sbopkg/SBo/15.0/network/curlie
info:   Curl wrapper
`
	packages := ParseSbofindOutput(output)
	if len(packages) != 1 || packages[0].Name != "curlie" || packages[0].Description != "Curl wrapper" {
		t.Fatalf("got %+v", packages)
	}
}

func TestParseMirrorsFiltersByVersionAndTracksActiveState(t *testing.T) {
	content := `
# comment
https://mirror1.example/slackware64-15.0/
# https://mirror2.example/slackware64-15.0/
https://mirror3.example/slackware64-current/
`
	mirrors := ParseMirrorsFromContent(content, "slackware64-15.0")
	if len(mirrors) != 2 || !mirrors[0].IsActive || mirrors[1].IsActive {
		t.Fatalf("got %+v", mirrors)
	}
	if mirrors[0].URL != "https://mirror1.example/slackware64-15.0/" {
		t.Fatalf("got %q", mirrors[0].URL)
	}
}

func TestExtractRegion(t *testing.T) {
	cases := map[string]string{
		"https://mirrors.kernel.org/slackware/":      "KERNEL",
		"http://ftp.osuosl.org/pub/slackware/":       "US",
		"https://slackware.uk/slackware/":            "UK",
		"ftp://ftp.example.de/pub/":                  "DE",
		"https://example.com/de/slackware64-current": "DE",
		"https://example.com/slackware64-current":    "Unknown",
	}
	for in, want := range cases {
		if got := extractRegion(in); got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
}

func TestRewriteMirrorsContentEnablesExactlyOneMirror(t *testing.T) {
	content := `
# Slackware mirrors
https://mirror1.example/slackware64-15.0/
# https://mirror2.example/slackware64-15.0/
`
	rewritten, err := RewriteMirrorsContent(content, "https://mirror2.example/slackware64-15.0/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rewritten, "# https://mirror1.example/slackware64-15.0/") {
		t.Fatal(rewritten)
	}
	active := 0
	for _, line := range strings.Split(rewritten, "\n") {
		if strings.HasPrefix(line, "https://") {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("active=%d", active)
	}
	if _, err := RewriteMirrorsContent(content, "https://missing.example/"); err == nil ||
		err.Error() != "Configuration error: Mirror 'https://missing.example/' was not found in mirrors file" {
		t.Fatalf("got %v", err)
	}
}
