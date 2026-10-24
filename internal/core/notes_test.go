package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNotesExcerpt(t *testing.T) {
	old := notesDir
	notesDir = t.TempDir()
	defer func() { notesDir = old }()
	if NotesExcerpt("my-app", 2) != nil {
		t.Fatal("want nil when no notes")
	}
	os.WriteFile(filepath.Join(notesDir, "my-app.md"), []byte("\n# Title\n\n  body line \nthird\n"), 0o644)
	got := NotesExcerpt("my-app", 2)
	if len(got) != 2 || got[0] != "# Title" || got[1] != "body line" {
		t.Fatal(got)
	}
}
