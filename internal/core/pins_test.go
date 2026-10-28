package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPins(t *testing.T) {
	old := notesDir
	defer func() { notesDir = old }()
	notesDir = ""
	if err := SetPinned("my-app", true); err == nil {
		t.Error("pinned with no data folder")
	}
	if len(ReadPins()) != 0 {
		t.Error("pins with no data folder")
	}

	base := t.TempDir()
	notesDir = filepath.Join(base, "notes")
	if len(ReadPins()) != 0 {
		t.Error("pins from a missing file")
	}
	for _, step := range []struct {
		name string
		on   bool
	}{{"my-app", true}, {"api-server", true}, {"my-app", true}, {"api-server", false}, {"demo", false}} {
		if err := SetPinned(step.name, step.on); err != nil {
			t.Fatal(err)
		}
	}
	if p := ReadPins(); len(p) != 1 || !p["my-app"] {
		t.Errorf("pins %v, want only my-app", p)
	}
	if PinsPath() != filepath.Join(base, "pins.json") {
		t.Errorf("path %s", PinsPath())
	}
	for _, bad := range []string{"", "..", "a/b"} {
		if SetPinned(bad, true) == nil {
			t.Errorf("pinned %q", bad)
		}
	}

	// a file mid-edit by hand is left alone
	os.WriteFile(PinsPath(), []byte(`{"pinned": [`), 0o644)
	if SetPinned("demo", true) == nil {
		t.Error("saved over invalid JSON")
	}
	if b, _ := os.ReadFile(PinsPath()); string(b) != `{"pinned": [` {
		t.Errorf("file changed: %s", b)
	}
	if len(ReadPins()) != 0 {
		t.Error("pins from invalid JSON")
	}
}
