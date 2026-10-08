package cron

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func sys(p string) cronSource { return cronSource{sourceSystem, p} }

func TestParseCronLineSkips(t *testing.T) {
	for _, line := range []string{"", "   ", "# comment", "  #x 1 2 3 4 5", "PATH=/usr/bin", "SHELL=/bin/sh", "1 2 3 4 5", "@bogus cmd"} {
		if _, ok := parseCronLine(line, sys("/x")); ok {
			t.Errorf("%q should be skipped", line)
		}
	}
}

func TestParseCronLineSpecial(t *testing.T) {
	cases := []struct {
		line                 string
		m, h, d, mo, wd, cmd string
	}{
		{"@reboot /bin/run a", "@reboot", "-", "-", "-", "-", "/bin/run a"},
		{"@yearly y", "0", "0", "1", "1", "*", "y"},
		{"@annually y", "0", "0", "1", "1", "*", "y"},
		{"@monthly m", "0", "0", "1", "*", "*", "m"},
		{"@weekly w", "0", "0", "*", "*", "0", "w"},
		{"@daily d", "0", "0", "*", "*", "*", "d"},
		{"@midnight d", "0", "0", "*", "*", "*", "d"},
		{"@hourly h", "0", "*", "*", "*", "*", "h"},
		{"@hourly", "0", "*", "*", "*", "*", ""},
	}
	for _, c := range cases {
		job, ok := parseCronLine("  "+c.line+"  ", sys("/etc/crontab"))
		if !ok {
			t.Fatalf("%q not parsed", c.line)
		}
		got := []string{job.Minute, job.Hour, job.Day, job.Month, job.Weekday, job.Command}
		want := []string{c.m, c.h, c.d, c.mo, c.wd, c.cmd}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%q: got %v want %v", c.line, got, want)
		}
		if job.RawLine != c.line || !job.Enabled {
			t.Errorf("%q: raw %q enabled %v", c.line, job.RawLine, job.Enabled)
		}
	}
}

func TestParseCronLineStandard(t *testing.T) {
	// System crontab: 6th field is the user.
	job, ok := parseCronLine("30 3 * * 0 root test -e  /x", sys("/etc/crontab"))
	if !ok || job.Command != "test -e /x" || job.Weekday != "0" || job.Minute != "30" {
		t.Fatalf("got %+v %v", job, ok)
	}
	job, ok = parseCronLine("30 3 * * 0 root test", sys("/etc/cron.d/foo"))
	if !ok || job.Command != "test" {
		t.Fatalf("got %+v %v", job, ok)
	}
	// Only a user: no command.
	if _, ok := parseCronLine("30 3 * * 0 root", sys("/etc/cron.d/foo")); ok {
		t.Fatal("expected no job")
	}
	// User crontab: no user field.
	job, ok = parseCronLine("*/5 * * * * echo hi", cronSource{sourceUser, "bob"})
	if !ok || job.Command != "echo hi" || job.Minute != "*/5" {
		t.Fatalf("got %+v %v", job, ok)
	}
	// Contains '=' and spaces: not a variable assignment.
	job, ok = parseCronLine("0 1 * * * FOO=1 run", cronSource{sourceUser, "bob"})
	if !ok || job.Command != "FOO=1 run" {
		t.Fatalf("got %+v %v", job, ok)
	}
}

func TestFormatSchedule(t *testing.T) {
	cases := []struct {
		job  cronJob
		want string
	}{
		{cronJob{Minute: "@reboot"}, "At reboot"},
		{cronJob{Minute: "*", Hour: "*", Day: "*", Month: "*", Weekday: "*"}, "Every hour"},
		{cronJob{Minute: "0", Hour: "0", Day: "*", Month: "*", Weekday: "0"}, "0:00 Sun"},
		{cronJob{Minute: "30", Hour: "3", Day: "*", Month: "*", Weekday: "7"}, ":30 3:00 Sun"},
		{cronJob{Minute: "09,39", Hour: "*", Day: "1", Month: "*", Weekday: "1-5"}, ":09,39 Every hour day 1 1-5"},
		{cronJob{Minute: "0", Hour: "0", Day: "1", Month: "1", Weekday: "6"}, "0:00 day 1 Sat"},
	}
	for _, c := range cases {
		if got := formatSchedule(&c.job); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}

func TestSourceDisplay(t *testing.T) {
	cases := []struct {
		src  cronSource
		name string
		col  tui.Color
	}{
		{sys("/etc/cron.hourly/x"), "hourly", tui.Cyan},
		{sys("/etc/cron.daily/x"), "daily", tui.Green},
		{sys("/etc/cron.weekly/x"), "weekly", tui.Yellow},
		{sys("/etc/cron.monthly/x"), "monthly", tui.Magenta},
		{sys("/etc/crontab"), "system", tui.Blue},
		{cronSource{sourceUser, "bob"}, "bob", tui.White},
	}
	for _, c := range cases {
		n, col := sourceDisplay(c.src)
		if n != c.name || col != c.col {
			t.Errorf("%v: got %s %v", c.src, n, col)
		}
	}
	if got := sourcePath(cronSource{sourceUser, "bob"}); got != "/var/spool/cron/crontabs/bob" {
		t.Error(got)
	}
}

func TestTruncateCommand(t *testing.T) {
	short := strings.Repeat("a", 60)
	if truncateCommand(short) != short {
		t.Error("60 bytes must not be truncated")
	}
	long := strings.Repeat("b", 61)
	if got := truncateCommand(long); got != strings.Repeat("b", 57)+"..." {
		t.Error(got)
	}
	// Multi-byte char straddling byte 57: cut at the previous boundary.
	mb := strings.Repeat("c", 56) + "é" + strings.Repeat("d", 10)
	if got := truncateCommand(mb); got != strings.Repeat("c", 56)+"..." {
		t.Error(got)
	}
}

func withPaths(t *testing.T, root string) {
	t.Helper()
	saved := []string{cronHourlyDir, cronDailyDir, cronWeeklyDir, cronMonthlyDir, systemCrontab, cronDDir, userCrontabsDir}
	cronHourlyDir = filepath.Join(root, "cron.hourly")
	cronDailyDir = filepath.Join(root, "cron.daily")
	cronWeeklyDir = filepath.Join(root, "cron.weekly")
	cronMonthlyDir = filepath.Join(root, "cron.monthly")
	systemCrontab = filepath.Join(root, "crontab")
	cronDDir = filepath.Join(root, "cron.d")
	userCrontabsDir = filepath.Join(root, "crontabs")
	t.Cleanup(func() {
		cronHourlyDir, cronDailyDir, cronWeeklyDir, cronMonthlyDir = saved[0], saved[1], saved[2], saved[3]
		systemCrontab, cronDDir, userCrontabsDir = saved[4], saved[5], saved[6]
	})
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAndFilter(t *testing.T) {
	root := t.TempDir()
	withPaths(t, root)
	write(t, filepath.Join(root, "cron.hourly", "h1"), "x", 0o755)
	write(t, filepath.Join(root, "cron.daily", "d1"), "x", 0o644)
	write(t, filepath.Join(root, "cron.daily", "d1~"), "x", 0o755)
	write(t, filepath.Join(root, "cron.daily", ".hidden"), "x", 0o755)
	write(t, filepath.Join(root, "cron.weekly", "w1"), "x", 0o700)
	if err := os.MkdirAll(filepath.Join(root, "cron.monthly", "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "crontab"), "SHELL=/bin/sh\n# c\n17 * * * * run-parts\n@reboot boot\n", 0o644)
	write(t, filepath.Join(root, "crontabs", "alice"), "0 5 1 * * backup now\r\n", 0o600)
	write(t, filepath.Join(root, "crontabs", "bad"), "\xff\xfe 1 2 3 4 5\n", 0o600)

	c := New()
	if len(c.jobs) != 6 {
		for _, j := range c.jobs {
			t.Logf("%+v", j)
		}
		t.Fatalf("got %d jobs", len(c.jobs))
	}
	if sel, ok := c.listState.Selected(); !ok || sel != 0 {
		t.Fatal("first job should be selected")
	}

	byCmd := map[string]cronJob{}
	for _, j := range c.jobs {
		byCmd[j.Command] = j
	}
	if !byCmd["h1"].Enabled || byCmd["d1"].Enabled || !byCmd["w1"].Enabled {
		t.Error("executable bits not honoured")
	}
	if j := byCmd["h1"]; j.Hour != "*" || j.RawLine != "@hourly h1" || j.Source.Value != filepath.Join(root, "cron.hourly")+"/h1" {
		t.Errorf("%+v", j)
	}
	if j := byCmd["w1"]; j.Hour != "0" || j.Weekday != "0" {
		t.Errorf("%+v", j)
	}
	// The temp crontab is not /etc/crontab, so the 6th field is the command.
	if _, ok := byCmd["run-parts"]; !ok {
		t.Error("crontab line not loaded")
	}
	if j := byCmd["backup now"]; j.Source.Kind != sourceUser || j.Source.Value != "alice" || j.RawLine != "0 5 1 * * backup now" {
		t.Errorf("%+v", j)
	}

	counts := map[cronFilter]int{}
	for f := filterAll; f <= filterMonthly; f++ {
		c.filter = f
		counts[f] = len(c.filteredJobs())
	}
	want := map[cronFilter]int{
		filterAll: 6, filterSystem: 5, filterUser: 1,
		filterHourly:  1, // h1 (raw line contains "hourly")
		filterDaily:   2, // d1, w1 (hour 0, day *)
		filterWeekly:  2, // w1, @reboot (weekday "-")
		filterMonthly: 1, // backup now (day 1, month *)
	}
	for f, n := range want {
		if counts[f] != n {
			t.Errorf("filter %d: got %d want %d", f, counts[f], n)
		}
	}
}

func TestHandleInput(t *testing.T) {
	root := t.TempDir()
	withPaths(t, root)
	write(t, filepath.Join(root, "crontabs", "u"), "1 * * * * a\n2 * * * * b\n", 0o600)
	c := New()

	c.HandleInput(tui.NewKey(tui.KeyUp, 0))
	if sel, _ := c.listState.Selected(); sel != 0 {
		t.Fatal("up at top must stay")
	}
	c.HandleInput(tui.CharKey('j', 0))
	c.HandleInput(tui.NewKey(tui.KeyDown, 0))
	if sel, _ := c.listState.Selected(); sel != 1 {
		t.Fatalf("down past end: %d", sel)
	}
	c.HandleInput(tui.CharKey('k', 0))
	if sel, _ := c.listState.Selected(); sel != 0 {
		t.Fatal("k")
	}
	c.HandleInput(tui.NewKey(tui.KeyTab, 0))
	if c.filter != filterSystem {
		t.Fatal("tab")
	}
	for i := 0; i < 6; i++ {
		c.HandleInput(tui.NewKey(tui.KeyTab, 0))
	}
	if c.filter != filterAll {
		t.Fatal("tab wrap")
	}
	if m := c.HandleInput(tui.FKey(5, 0)); m != nil || c.statusMessage == nil || c.statusMessage.text != "Cron jobs refreshed" {
		t.Fatal("F5")
	}

	// Down with no selection selects the first item.
	c.listState.SelectNone()
	c.HandleInput(tui.NewKey(tui.KeyDown, 0))
	if sel, ok := c.listState.Selected(); !ok || sel != 0 {
		t.Fatal("down from none")
	}
}

func TestRenderEmptyDoesNotPanic(t *testing.T) {
	withPaths(t, t.TempDir())
	c := New()
	buf := tui.NewBuffer(tui.Rect{Width: 80, Height: 20})
	c.Render(&tui.Frame{Buf: buf}, buf.Area)
}
