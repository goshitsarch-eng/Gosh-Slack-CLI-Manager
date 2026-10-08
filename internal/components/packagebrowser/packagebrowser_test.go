package packagebrowser

import (
	"strings"
	"testing"
)

func TestParsePackageName(t *testing.T) {
	pkg, ok := parsePackageName("kernel-generic-5.15.19-x86_64-1")
	if !ok || pkg.name != "kernel-generic" || pkg.version != "5.15.19" || pkg.arch != "x86_64" || pkg.build != "1" {
		t.Fatalf("unexpected parse: %+v %v", pkg, ok)
	}
	if _, ok := parsePackageName("two-dashes-only"); ok {
		t.Fatal("expected failure for a name with too few dashes")
	}
}

func TestExtractDescription(t *testing.T) {
	content := "PACKAGE NAME: bash\nPACKAGE DESCRIPTION:\nbash: bash (sh-compatible shell)\nbash:\nbash: The GNU shell.\nFILE LIST:\nbin/bash\n"
	if got := extractDescription(content); got != "bash (sh-compatible shell) The GNU shell." {
		t.Fatalf("got %q", got)
	}
	long := "PACKAGE DESCRIPTION:\nx: " + strings.Repeat("a", 250) + "\n"
	if got := extractDescription(long); len(got) != 203 || !strings.HasSuffix(got, "...") {
		t.Fatalf("long description not truncated: %d", len(got))
	}
}

func TestExtractSize(t *testing.T) {
	content := "COMPRESSED PACKAGE SIZE:     2.1M\nUNCOMPRESSED PACKAGE SIZE: 8.0M\n"
	if got := extractSize(content, "COMPRESSED PACKAGE SIZE:"); got != "2.1M" {
		t.Fatalf("got %q", got)
	}
	if got := extractSize(content, "MISSING:"); got != "Unknown" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyFilterSelection(t *testing.T) {
	c := &Component{packages: []installedPackage{{name: "bash", description: "shell"}, {name: "zlib"}}}
	c.searchQuery = "SHELL"
	c.applyFilter()
	if len(c.filteredPackages) != 1 || c.filteredPackages[0] != 0 {
		t.Fatalf("filtered = %v", c.filteredPackages)
	}
	c.searchQuery = "nothing"
	c.applyFilter()
	if _, ok := c.listState.Selected(); ok {
		t.Fatal("selection should be cleared when nothing matches")
	}
}
