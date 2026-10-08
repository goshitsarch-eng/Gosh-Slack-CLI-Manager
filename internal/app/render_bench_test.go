package app

import (
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/components"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/slackware"
	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func benchmarkRender(b *testing.B, tab components.Tab) {
	a := New(slackware.Version{Kind: slackware.Current}, false)
	a.SwitchToTab(tab)
	buf := tui.NewBuffer(tui.Rect{Width: 140, Height: 45})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.ResetAll()
		a.Render(&tui.Frame{Buf: buf})
	}
}

func BenchmarkRenderUpdater(b *testing.B) { benchmarkRender(b, components.TabUpdater) }
func BenchmarkRenderSysInfo(b *testing.B) { benchmarkRender(b, components.TabSysInfo) }
func BenchmarkRenderLogs(b *testing.B)    { benchmarkRender(b, components.TabLogs) }
