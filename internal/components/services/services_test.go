package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServicesFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("rc.M", "#!/bin/sh\n", 0o755)
	write("rc.local_shutdown", "#!/bin/sh\n", 0o755)
	write("rc.foo~", "#!/bin/sh\n", 0o755)
	write("rc.foo.new", "#!/bin/sh\n", 0o755)
	write("rc.zeta", "#!/bin/sh\n# Zeta daemon service here\n", 0o755)
	write("rc.alpha", "#!/bin/sh\n# short\n# has/a/slash in it\n", 0o644)
	write("rc.x", "#!/bin/sh\n", 0o644)
	write("other", "#!/bin/sh\n", 0o755)

	got := loadServicesFrom(dir)
	var names []string
	for _, s := range got {
		names = append(names, s.name)
	}
	want := []string{"rc.alpha", "rc.x", "rc.zeta"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
	if got[0].isEnabled || !got[2].isEnabled {
		t.Fatalf("enabled flags wrong: %+v", got)
	}
	if got[0].description != "No description available" || got[2].description != "Zeta daemon service here" {
		t.Fatalf("descriptions wrong: %q %q", got[0].description, got[2].description)
	}
}

func TestTrimStartMatches(t *testing.T) {
	if got := trimStartMatches("rc.rc.foo", "rc."); got != "foo" {
		t.Fatalf("got %q", got)
	}
}
