package core

import (
	"path/filepath"
	"testing"
)

func TestTUIPrefsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "tui.json")
	if got := ReadTUIPrefs(p); got.Theme != "" || got.Colors != nil {
		t.Errorf("missing file gave %+v", got)
	}
	want := TUIPrefs{HideHints: true, Theme: "ink", StatusDot: true, Colors: map[string]string{"my-app": "#E05A8A"}}
	if err := WriteTUIPrefs(p, want); err != nil {
		t.Fatal(err)
	}
	got := ReadTUIPrefs(p)
	if got.Theme != "ink" || !got.HideHints || !got.StatusDot || got.Colors["my-app"] != "#E05A8A" {
		t.Errorf("got %+v", got)
	}
}
