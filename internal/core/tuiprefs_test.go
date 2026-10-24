package core

import (
	"os"
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

func TestUpdateTUIPrefsKeepsOthers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tui.json")
	// two TUIs: A colours a repo, then B, which read the file before that, hides the hints
	if err := UpdateTUIPrefs(p, func(x *TUIPrefs) { x.Colors = map[string]string{"my-app": "#E05A8A"} }); err != nil {
		t.Fatal(err)
	}
	if err := UpdateTUIPrefs(p, func(x *TUIPrefs) { x.HideHints = true }); err != nil {
		t.Fatal(err)
	}
	if got := ReadTUIPrefs(p); got.Colors["my-app"] != "#E05A8A" || !got.HideHints {
		t.Errorf("got %+v", got)
	}
}

func TestUpdateTUIPrefsLeavesBadFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tui.json")
	bad := []byte(`{"theme": "ink",`)
	os.WriteFile(p, bad, 0o600)
	if err := UpdateTUIPrefs(p, func(x *TUIPrefs) { x.HideHints = true }); err == nil {
		t.Error("no error for a broken file")
	}
	if b, _ := os.ReadFile(p); string(b) != string(bad) {
		t.Errorf("broken file was overwritten: %s", b)
	}
}
