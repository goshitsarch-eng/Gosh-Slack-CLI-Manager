package prefs

import (
	"encoding/json"
	"os"
	"testing"
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
