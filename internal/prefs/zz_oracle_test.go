package prefs

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/tui"
)

func TestZZOracleDump(t *testing.T) {
	in, out := os.Getenv("ORACLE_IN"), os.Getenv("ORACLE_OUT")
	if in == "" {
		t.Skip()
	}
	data, _ := os.ReadFile(in)
	var docs []string
	if err := json.Unmarshal(data, &docs); err != nil {
		t.Fatal(err)
	}
	var res []map[string]any
	for _, d := range docs {
		s, err := decodeSettings(d)
		if err != nil {
			res = append(res, map[string]any{"err": err.Error()})
		} else {
			res = append(res, map[string]any{"ok": []any{int(s.Theme), s.ConfirmActions, s.ShowHiddenFiles, s.AutoRefresh, s.RefreshInterval, s.DefaultTab, s.LogLines}})
		}
	}
	b, _ := json.Marshal(res)
	os.WriteFile(out, b, 0o644)
}

func TestZZOracleSer(t *testing.T) {
	in, out := os.Getenv("ORACLE_SER_IN"), os.Getenv("ORACLE_OUT")
	if in == "" {
		t.Skip()
	}
	data, _ := os.ReadFile(in)
	var docs [][]any
	if err := json.Unmarshal(data, &docs); err != nil {
		t.Fatal(err)
	}
	var res []map[string]any
	for _, d := range docs {
		s := AppSettings{Theme: ThemeChoice(d[0].(float64)), ConfirmActions: d[1].(bool), ShowHiddenFiles: d[2].(bool), AutoRefresh: d[3].(bool),
			RefreshInterval: uint32(d[4].(float64)), DefaultTab: d[5].(string), LogLines: int(d[6].(float64))}
		res = append(res, map[string]any{"ok": encodeSettings(s)})
	}
	b, _ := json.Marshal(res)
	os.WriteFile(out, b, 0o644)
}

func TestZZRender(t *testing.T) {
	in, out := os.Getenv("RENDER_IN"), os.Getenv("ORACLE_OUT")
	if in == "" {
		t.Skip()
	}
	data, _ := os.ReadFile(in)
	var msgs []string
	json.Unmarshal(data, &msgs)
	var res [][]string
	for _, m := range msgs {
		buf := tui.NewBuffer(tui.Rect{Width: 120, Height: 1})
		tui.ParagraphLine(tui.LineSpan(tui.Styled(m, tui.NewStyle().FG(tui.Red)))).Render(tui.Rect{Width: 120, Height: 1}, buf)
		var cells []string
		for x := 0; x < 120; x++ {
			cells = append(cells, buf.Cell(x, 0).Symbol)
		}
		res = append(res, cells)
	}
	b, _ := json.Marshal(res)
	os.WriteFile(out, b, 0o644)
}
