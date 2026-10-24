package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ezjones/repodash/internal/core"
)

func band(r core.Repo, inner, spin int) string {
	return strings.Split(renderCard(r, r.Name, nil, false, inner, time.Unix(1000, 0), spin), "\n")[1]
}

func TestAgentBadgeInBand(t *testing.T) {
	busy := band(core.Repo{Name: "my-app", Level: "ok", Claude: "busy"}, 30, 3)
	if !strings.Contains(busy, spinFrames[3]) {
		t.Errorf("busy band has no spinner frame: %q", busy)
	}
	if !strings.Contains(band(core.Repo{Name: "my-app", Level: "ok", Claude: "busy"}, 30, 4), spinFrames[4]) {
		t.Error("frame did not follow spin")
	}
	wait := band(core.Repo{Name: "my-app", Level: "ok", Claude: "waiting"}, 30, 0)
	if !strings.Contains(wait, "●") || strings.Contains(wait, spinFrames[0]) {
		t.Errorf("waiting band: %q", wait)
	}
	if strings.Contains(band(core.Repo{Name: "my-app", Level: "ok"}, 30, 0), spinFrames[0]) {
		t.Error("spinner without an agent")
	}
}

func TestAgentBadgeNarrowCard(t *testing.T) {
	// no pill label and no badge fit; the icon still shows next to the status dot
	b := band(core.Repo{Name: "my-application", Level: "ok", Claude: "busy", Language: "Go"}, 12, 2)
	if !strings.Contains(b, spinFrames[2]) {
		t.Errorf("narrow band lost the icon: %q", b)
	}
}

func TestSpinTickOnlyWhileBusy(t *testing.T) {
	f := newFakeTUI(t, false)
	if f.m.spinCmd() != nil {
		t.Fatal("spin tick started with nothing busy")
	}
	if cmd := f.send(tickMsg("spin")); cmd != nil || f.m.spin != 0 {
		t.Fatalf("idle spin tick: cmd %v, spin %d", cmd != nil, f.m.spin)
	}
	f.m.repos[0].Claude = "busy"
	f.m.rebuild()
	if f.m.spinCmd() == nil || !f.m.spinPending {
		t.Fatal("busy repo did not start the spin tick")
	}
	if f.m.spinCmd() != nil {
		t.Fatal("a second spin tick was started while one is pending")
	}
	if cmd := f.send(tickMsg("spin")); cmd == nil || f.m.spin != 1 {
		t.Fatalf("busy spin tick: cmd %v, spin %d", cmd != nil, f.m.spin)
	}
	f.m.repos[0].Claude = "idle"
	f.m.rebuild()
	if cmd := f.send(tickMsg("spin")); cmd != nil || f.m.spin != 1 || f.m.spinPending {
		t.Fatalf("after idle: cmd %v, spin %d, pending %v", cmd != nil, f.m.spin, f.m.spinPending)
	}
	// a panes update that shows a busy repo restarts it
	f.m.repos[0].Claude = "busy"
	f.m.rebuild()
	if f.send(tickMsg("panes")) == nil || !f.m.spinPending {
		t.Fatal("spin tick not restarted")
	}
}

func TestInspectorAgentLine(t *testing.T) {
	f, _ := panelFake(t)
	f.selectName(t, "my-app")
	f.m.view[f.m.sel].Claude = "busy"
	f.m.spin = 5
	if v := f.m.View().Content; !strings.Contains(v, spinFrames[5]+" working") {
		t.Errorf("no spinner in the agent line:\n%s", v)
	}
}

func TestInspectorShowsAllNotes(t *testing.T) {
	f, _ := panelFake(t)
	var lines []string
	for i := 1; i <= 12; i++ {
		lines = append(lines, "note-line-"+string(rune('a'+i-1)))
	}
	text := strings.Join(lines[:6], "\n") + "\n\n\n\n" + strings.Join(lines[6:], "\n") + "\n"
	f.m.d.Notes = func(string) string { return text }
	f.selectName(t, "my-app")
	got := noteLines(text, 40)
	if len(got) != 13 { // 12 lines and one single blank
		t.Fatalf("noteLines = %d lines: %q", len(got), got)
	}
	pl := strings.Join(panelLines(f.m, 44), "\n")
	for _, l := range lines {
		if !strings.Contains(pl, l) {
			t.Errorf("%s missing from the panel", l)
		}
	}
	if strings.Contains(pl, "n: add notes") {
		t.Error("hint shown although notes exist")
	}
}

func TestInspectorNotesHint(t *testing.T) {
	f, _ := panelFake(t)
	f.m.d.Notes = func(string) string { return " \n\n" }
	pl := strings.Join(panelLines(f.m, 44), "\n")
	if !strings.Contains(pl, "NOTES") || !strings.Contains(pl, "n: add notes") {
		t.Errorf("no hint:\n%s", pl)
	}
}

func TestNoteLinesWrap(t *testing.T) {
	got := noteLines("  - aaa bbb ccc ddd eee fff", 12)
	if len(got) < 2 {
		t.Fatalf("not wrapped: %q", got)
	}
	for _, l := range got {
		if len(l) > 12 {
			t.Errorf("line too wide: %q", l)
		}
	}
}

func TestEditKeyRunsEditorOnNotesPath(t *testing.T) {
	dir := t.TempDir()
	core.SetDataDirs(filepath.Join(dir, "img"), filepath.Join(dir, "notes"))
	f := newFakeTUI(t, false)
	var gotPath string
	f.m.d.EditCmd = func(path string) *exec.Cmd {
		gotPath = path
		return exec.Command("true", path)
	}
	f.selectName(t, "api-server")
	if f.keys("e") == nil {
		t.Fatal("e returned no command")
	}
	want := filepath.Join(dir, "notes", "api-server.md")
	if gotPath != want {
		t.Errorf("editor path %q, want %q", gotPath, want)
	}
	if fi, err := os.Stat(filepath.Dir(want)); err != nil || !fi.IsDir() {
		t.Error("notes folder not created")
	}
	f.m.d.EditCmd = func(string) *exec.Cmd { return nil }
	if f.keys("e") != nil || !strings.Contains(f.m.msg, "no editor") {
		t.Errorf("missing editor: msg %q", f.m.msg)
	}
}

func TestEmptyNotesRemovedAfterEdit(t *testing.T) {
	dir := t.TempDir()
	core.SetDataDirs(filepath.Join(dir, "img"), filepath.Join(dir, "notes"))
	p := core.NotesPath("my-app")
	os.MkdirAll(filepath.Dir(p), 0o755)
	f := newFakeTUI(t, false)

	os.WriteFile(p, []byte("  \n\n"), 0o644)
	f.send(editedMsg{name: "my-app"})
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("blank notes file kept")
	}
	os.WriteFile(p, []byte("keep me\n"), 0o644)
	f.send(editedMsg{name: "my-app"})
	if _, err := os.Stat(p); err != nil || f.m.msg != "notes saved" {
		t.Errorf("real notes: err %v, msg %q", err, f.m.msg)
	}
	if core.NotesPath("../x") != "" || core.NotesPath("") != "" {
		t.Error("bad names accepted")
	}
}
