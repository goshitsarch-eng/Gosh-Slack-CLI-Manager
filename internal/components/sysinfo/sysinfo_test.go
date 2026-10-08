package sysinfo

import (
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func TestFormatBytes(t *testing.T) {
	cases := map[uint64]string{
		0:               "0 B",
		1023:            "1023 B",
		1024:            "1.00 KB",
		1536:            "1.50 KB",
		5 * 1024 * 1024: "5.00 MB",
		16879083520:     "15.72 GB",
		1 << 40:         "1.00 TB",
	}
	for in, want := range cases {
		if got := formatBytes(in); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatUptime(t *testing.T) {
	cases := map[uint64]string{59: "0m", 61: "1m", 3600: "1h 0m", 90061: "1d 1h 1m"}
	for in, want := range cases {
		if got := formatUptime(in); got != want {
			t.Errorf("formatUptime(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestCPUUsageBetweenRefreshes(t *testing.T) {
	u := newCPUUsage([10]uint64{100, 0, 100, 800})
	if u.percent != 0 {
		t.Fatalf("first sample percent = %v, want 0", u.percent)
	}
	u.set([10]uint64{150, 0, 150, 900}) // +100 work, +200 total
	if u.percent != 50 {
		t.Fatalf("percent = %v, want 50", u.percent)
	}
}

func TestRefreshHonoursMinimumInterval(t *testing.T) {
	var w cpusWrapper
	w.refresh()
	if len(w.cpus) == 0 {
		t.Skip("no /proc/stat cpu lines")
	}
	before := w.lastUpdate
	w.refresh()
	if !w.lastUpdate.Equal(before) {
		t.Fatal("refresh within the minimum interval must not re-read CPU times")
	}
}

func TestUsageColor(t *testing.T) {
	if usageColor(90) != tui.Red || usageColor(70) != tui.Yellow || usageColor(69) != tui.Green {
		t.Fatal("unexpected usage colors")
	}
}
