package disks

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

const dfOut = `Filesystem              Mounted on   Type     1B-blocks        Used       Avail Use%
tmpfs                   /dev/shm     tmpfs  16876494848           0 16876494848   0%
/dev/vda                /            ext4  270553174016 12542185472 29281673216  30%
/dev/sdb1               /data        xfs      281075712   250667008    24768512  92%
/dev/sdc                /x           ext4     bogus        1           2         -
`

const lsblkOut = `zram0            0 disk
vda   274877906944 disk        /
sdb   300000000000 disk
├─sdb1   281075712 part xfs    /data
└─sdb2      123456 part ext4
loop0       4096 loop squashfs /snap
loop1       4096 disk
sr0   1073741824 rom
`

func withFakes(t *testing.T, df, lsblk string, dfErr error) {
	t.Helper()
	sd, sl := runDf, runLsblk
	runDf = func() ([]byte, error) { return []byte(df), dfErr }
	runLsblk = func() ([]byte, error) { return []byte(lsblk), nil }
	t.Cleanup(func() { runDf, runLsblk = sd, sl })
}

func TestLoadDiskInfo(t *testing.T) {
	withFakes(t, dfOut, lsblkOut, nil)
	c := New()
	var names []string
	for _, d := range c.disks {
		names = append(names, d.Name)
	}
	want := []string{"sdb", "sdb1", "sdb2", "sdc", "vda", "zram0"}
	if len(names) != len(want) {
		t.Fatalf("got %v want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v want %v", names, want)
		}
	}
	byName := map[string]diskInfo{}
	for _, d := range c.disks {
		byName[d.Name] = d
	}
	if d := byName["vda"]; !d.IsMounted || d.UsePercent != 30 || d.Size != 270553174016 || d.MountPoint != "/" || d.DevicePath != "/dev/vda" {
		t.Errorf("%+v", d)
	}
	if d := byName["sdc"]; d.Size != 0 || d.Used != 1 || d.UsePercent != 0 {
		t.Errorf("%+v", d)
	}
	if d := byName["sdb2"]; d.IsMounted || d.HasMountPoint || d.Filesystem != "ext4" || d.Available != 123456 {
		t.Errorf("%+v", d)
	}
	// Empty FSTYPE collapses: the mount point lands in the filesystem column.
	if d := byName["zram0"]; d.Filesystem != "" || d.HasMountPoint {
		t.Errorf("%+v", d)
	}
	if sel, ok := c.listState.Selected(); !ok || sel != 0 {
		t.Error("first disk should be selected")
	}
}

func TestCommandErrors(t *testing.T) {
	// A spawn failure discards the output.
	withFakes(t, dfOut, "", &os.PathError{})
	c := New()
	if len(c.disks) != 0 {
		t.Fatal("spawn failure must yield no disks")
	}

	// A non-zero exit status still uses stdout, like Command::output().
	runDf = func() ([]byte, error) { return []byte(dfOut), &exec.ExitError{} }
	c.loadDiskInfo()
	if len(c.disks) != 3 {
		t.Fatalf("expected 3 disks, got %d", len(c.disks))
	}
}

func TestFormatSize(t *testing.T) {
	cases := map[uint64]string{
		0:                    "0 B",
		1023:                 "1023 B",
		1024:                 "1.0 KB",
		1536:                 "1.5 KB",
		1048576:              "1.0 MB",
		282173440:            "269.1 MB",
		274877906944:         "256.0 GB",
		1099511627776 * 3:    "3.0 TB",
		1099511627776 * 2048: "2048.0 TB",
	}
	for in, want := range cases {
		if got := formatSize(in); got != want {
			t.Errorf("%d: got %q want %q", in, got, want)
		}
	}
}

func TestUsageColor(t *testing.T) {
	if usageColor(90) != tui.Red || usageColor(89) != tui.Yellow || usageColor(75) != tui.Yellow || usageColor(74) != tui.Green {
		t.Fail()
	}
}

func TestParseUint(t *testing.T) {
	for in, want := range map[string]uint64{"5": 5, "+5": 5, "255": 255} {
		if v, ok := parseUint(in, 8); !ok || v != want {
			t.Errorf("%q", in)
		}
	}
	for _, in := range []string{"", "+", "-1", "256", "++1", "+-1", "1_0", " 1"} {
		if _, ok := parseUint(in, 8); ok {
			t.Errorf("%q should fail", in)
		}
	}
}

func TestFindMountPoint(t *testing.T) {
	saved := fstabPath
	t.Cleanup(func() { fstabPath = saved })
	fstabPath = filepath.Join(t.TempDir(), "fstab")
	if err := os.WriteFile(fstabPath, []byte("# /dev/sda1 /nope\n\n  /dev/sda1   /boot vfat defaults 0 0\n/dev/sdb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Component{}
	if got := c.FindMountPoint("/dev/sda1"); got != "/boot" {
		t.Error(got)
	}
	if got := c.FindMountPoint("/dev/sdb"); got != "/mnt/sdb" {
		t.Error(got)
	}
	if got := c.FindMountPoint("/dev//dev/x"); got != "/mnt/x" {
		t.Error(got)
	}
	fstabPath = filepath.Join(t.TempDir(), "missing")
	if got := c.FindMountPoint("sdc"); got != "/mnt/sdc" {
		t.Error(got)
	}
}

func TestActionStartedAndStatus(t *testing.T) {
	c := &Component{}
	for _, tc := range []struct {
		a    msg.DiskAction
		want string
	}{
		{msg.DiskAction{Kind: msg.DiskMount, Target: "/dev/sdb1"}, "Mounting /dev/sdb1..."},
		{msg.DiskAction{Kind: msg.DiskUnmount, Target: "/data"}, "Unmounting /data..."},
		{msg.DiskAction{Kind: msg.DiskCheckFilesystem, Target: "/dev/sdb2"}, "Checking /dev/sdb2..."},
	} {
		c.ActionStarted(tc.a)
		if c.statusMessage == nil || c.statusMessage.text != tc.want || c.statusMessage.isError {
			t.Errorf("got %+v", c.statusMessage)
		}
	}
	c.SetStatus("boom", true)
	if !c.statusMessage.isError || c.statusMessage.text != "boom" {
		t.Fail()
	}
}

func TestHandleInput(t *testing.T) {
	withFakes(t, dfOut, lsblkOut, nil)
	c := New() // sdb, sdb1, sdb2, sdc, vda, zram0

	key := func(r rune) msg.Message { return c.HandleInput(tui.CharKey(r, 0)) }

	// sdb is unmounted: u does nothing, m asks for confirmation.
	key('u')
	if c.showConfirm {
		t.Fatal("u on unmounted disk")
	}
	key('m')
	if !c.showConfirm || c.pendingAction == nil || c.pendingAction.Kind != msg.DiskMount {
		t.Fatal("m")
	}
	// Other keys are swallowed while confirming.
	c.HandleInput(tui.NewKey(tui.KeyDown, 0))
	if sel, _ := c.listState.Selected(); sel != 0 {
		t.Fatal("down while confirming")
	}
	c.HandleInput(tui.NewKey(tui.KeyEsc, 0))
	if c.showConfirm || c.pendingAction != nil {
		t.Fatal("esc")
	}
	key('m')
	m := key('Y')
	if got, ok := m.(msg.DiskActionMsg); !ok || got.Action != (msg.DiskAction{Kind: msg.DiskMount, Target: "/dev/sdb"}) {
		t.Fatalf("got %#v", m)
	}

	// sdb1 is mounted at /data.
	key('j')
	key('m')
	if c.showConfirm {
		t.Fatal("m on mounted disk")
	}
	key('u')
	m = key('y')
	if got, ok := m.(msg.DiskActionMsg); !ok || got.Action != (msg.DiskAction{Kind: msg.DiskUnmount, Target: "/data"}) {
		t.Fatalf("got %#v", m)
	}
	key('f')
	if key('N') != nil || c.showConfirm {
		t.Fatal("N")
	}
	key('f')
	m = key('y')
	if got, ok := m.(msg.DiskActionMsg); !ok || got.Action != (msg.DiskAction{Kind: msg.DiskCheckFilesystem, Target: "/dev/sdb1"}) {
		t.Fatalf("got %#v", m)
	}

	for i := 0; i < 10; i++ {
		c.HandleInput(tui.NewKey(tui.KeyDown, 0))
	}
	if sel, _ := c.listState.Selected(); sel != 5 {
		t.Fatalf("down past end: %d", sel)
	}
	for i := 0; i < 10; i++ {
		key('k')
	}
	if sel, _ := c.listState.Selected(); sel != 0 {
		t.Fatal("up past top")
	}

	c.HandleInput(tui.NewKey(tui.KeyEnter, 0))
	if c.mode != modeDetails {
		t.Fatal("enter")
	}
	c.HandleInput(tui.NewKey(tui.KeyEnter, 0))
	if c.mode != modeOverview {
		t.Fatal("enter toggle")
	}
	c.HandleInput(tui.FKey(5, 0))
	if c.statusMessage == nil || c.statusMessage.text != "Disk info refreshed" {
		t.Fatal("F5")
	}
}

func TestRenderEmpty(t *testing.T) {
	withFakes(t, "", "", nil)
	c := New()
	c.HandleInput(tui.NewKey(tui.KeyEnter, 0))
	buf := tui.NewBuffer(tui.Rect{Width: 100, Height: 30})
	c.Render(&tui.Frame{Buf: buf}, buf.Area)
}
