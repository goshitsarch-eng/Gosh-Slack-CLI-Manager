package updater

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/ui"
)

func TestKernelDetectionMatchesKernelPackageNames(t *testing.T) {
	updater := New(slackware.Version{Kind: slackware.V15_0})

	if !updater.CheckForKernelUpdate("Upgrading kernel-generic-6.6.1") {
		t.Error("kernel-generic upgrade not detected")
	}
	if updater.CheckForKernelUpdate("Upgrading aaa_base-15.0") {
		t.Error("aaa_base upgrade detected as kernel")
	}
}

func TestBlacklistParserIgnoresCommentsAndBlankLines(t *testing.T) {
	entries := parseBlacklistEntries("# comment\n\nkernel-firmware\nmozilla-firefox\n")

	if want := []string{"kernel-firmware", "mozilla-firefox"}; !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %q, want %q", entries, want)
	}
}

func TestNewConfigScanFindsNewFiles(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "rc.d")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "rc.inet1.conf.new"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	found := scanNewConfigFiles(root, 8)

	if len(found) != 1 {
		t.Fatalf("found %d files, want 1: %q", len(found), found)
	}
	if !strings.HasSuffix(filepath.Base(found[0]), ".new") {
		t.Errorf("found %q, want a .new file", found[0])
	}
}

func TestLiloFlowWithKernelRequiresTypingSkip(t *testing.T) {
	c := New(slackware.Version{Kind: slackware.Current})
	c.startUpdate()
	c.bootloader = slackware.BootloaderLilo
	for range 3 {
		c.StepComplete(true, "")
	}
	c.SetKernelUpdated(true)
	c.StepComplete(true, "")
	if !c.NeedsLiloConfirm() {
		t.Fatal("expected lilo confirmation")
	}
	if _, _, ok := c.CurrentCommand(); ok {
		t.Fatal("no command expected while awaiting confirmation")
	}

	for _, r := range "sKx" {
		c.HandleInput(tui.CharKey(r, tui.ModNone))
	}
	if c.skipInput != "" {
		t.Fatalf("skip input = %q, want reset after a wrong letter", c.skipInput)
	}
	for _, r := range "SKI" {
		c.HandleInput(tui.CharKey(r, tui.ModNone))
	}
	c.HandleInput(tui.NewKey(tui.KeyBackspace, tui.ModNone))
	if c.skipInput != "SK" {
		t.Fatalf("skip input = %q, want SK", c.skipInput)
	}
	c.HandleInput(tui.CharKey('i', tui.ModNone))
	c.HandleInput(tui.CharKey('p', tui.ModNone))

	if c.NeedsLiloConfirm() || !c.IsShowingSummary() || !c.WasLiloSkipped() {
		t.Fatal("expected summary after typing SKIP")
	}
	if got := c.steps[4]; got.Status != ui.StepFailed || got.Error != "SKIPPED - KERNEL WAS UPDATED!" {
		t.Fatalf("lilo step = %+v", got)
	}
	c.HandleInput(tui.NewKey(tui.KeyEnter, tui.ModNone))
	if c.IsShowingSummary() {
		t.Fatal("Enter should dismiss the summary")
	}
}

func TestLiloConfirmContinuesUpdate(t *testing.T) {
	c := New(slackware.Version{Kind: slackware.V15_0})
	c.startUpdate()
	c.bootloader = slackware.BootloaderLilo
	for range 4 {
		c.StepComplete(true, "")
	}
	if m := c.HandleInput(tui.CharKey('y', tui.ModNone)); m != (msg.ContinueUpdate{}) {
		t.Fatalf("message = %#v, want ContinueUpdate", m)
	}
	cmd, args, ok := c.CurrentCommand()
	if !ok || cmd != "lilo" || len(args) != 0 {
		t.Fatalf("command = %q %q %v", cmd, args, ok)
	}
	c.StepComplete(true, "")
	if c.IsRunning() || !c.IsShowingSummary() || c.CurrentStep() != 5 {
		t.Fatal("expected finished update")
	}
}

func TestUnknownBootloaderFailsLiloStep(t *testing.T) {
	c := New(slackware.Version{Kind: slackware.Current})
	c.startUpdate()
	c.bootloader = slackware.BootloaderUnknown
	for range 4 {
		c.StepComplete(true, "")
	}
	if c.steps[4].Status != ui.StepFailed || c.steps[4].Error != "No bootloader detected" {
		t.Fatalf("lilo step = %+v", c.steps[4])
	}
	if c.IsRunning() || !c.IsShowingSummary() {
		t.Fatal("expected summary")
	}
}
