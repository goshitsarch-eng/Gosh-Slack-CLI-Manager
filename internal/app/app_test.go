package app

import (
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
)

func TestReadOnlyModeBlocksUpdateStart(t *testing.T) {
	app := New(slackware.Version{Kind: slackware.Current}, false)

	app.Update(msg.StartUpdate{})

	if app.Updater.IsRunning() {
		t.Error("updater should not be running in read-only mode")
	}
	if got, ok := app.Updater.LastOutput(); !ok || got != "Root privileges are required for system updates." {
		t.Errorf("last output = %q, %v", got, ok)
	}
}

func TestUpdaterProgressRoutesEvenWhenOtherTabIsActive(t *testing.T) {
	app := New(slackware.Version{Kind: slackware.Current}, true)
	app.SwitchToTab(components.TabLogs)

	app.Update(msg.TaskProgress{Target: msg.TargetUpdater, Line: "slackpkg update output"})

	if got, ok := app.Updater.LastOutput(); !ok || got != "slackpkg update output" {
		t.Errorf("last output = %q, %v", got, ok)
	}
}
