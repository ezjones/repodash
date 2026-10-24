package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type notesStore struct {
	text   map[string]string
	saved  []string
	saveFn int
}

func dialogFake(t *testing.T, text string) (*fakeTUI, *notesStore) {
	t.Helper()
	f := newFakeTUI(t, false)
	st := &notesStore{text: map[string]string{"my-app": text}}
	f.m.d.ReadNotes = func(n string) (string, bool) { t, ok := st.text[n]; return t, ok && t != "" }
	f.m.d.SaveNotes = func(n, tx string) error {
		st.saved = append(st.saved, tx)
		st.text[n] = tx
		return nil
	}
	f.m.d.Notes = func(n string) string { return st.text[n] }
	f.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	f.selectName(t, "my-app")
	return f, st
}

func (f *fakeTUI) ctrlS() { f.send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}) }

func TestDialogOpensWithNotes(t *testing.T) {
	f, _ := dialogFake(t, "first line\nsecond line")
	f.keys("n")
	if f.m.dlg == nil || f.m.dlg.edit {
		t.Fatal("n should open the dialog in view mode")
	}
	v := f.m.View().Content
	for _, w := range []string{"my-app notes", "first line", "second line", "e edit"} {
		if !strings.Contains(v, w) {
			t.Errorf("%q missing:\n%s", w, v)
		}
	}
}

func TestDialogEditSave(t *testing.T) {
	f, st := dialogFake(t, "old")
	f.keys("n", "e")
	if !f.m.dlg.edit {
		t.Fatal("e should edit")
	}
	f.keys("h", "e", "l", "l", "o")
	f.ctrlS()
	if len(st.saved) != 1 || !strings.Contains(st.saved[0], "hello") || !strings.HasPrefix(st.saved[0], "old") {
		t.Fatalf("saved %q", st.saved)
	}
	if f.m.dlg == nil || f.m.dlg.edit || f.m.msg != "notes saved" {
		t.Fatalf("after save: dlg %+v msg %q", f.m.dlg, f.m.msg)
	}
}

func TestDialogEscAsksThenDiscards(t *testing.T) {
	f, st := dialogFake(t, "old")
	f.keys("n", "e", "x", "esc")
	if !f.m.dlg.confirm || !strings.Contains(f.m.View().Content, "Save changes? y/n") {
		t.Fatal("esc after an edit should ask")
	}
	f.keys("esc")
	if f.m.dlg.confirm || !f.m.dlg.edit {
		t.Fatal("esc again keeps editing")
	}
	f.keys("esc", "n")
	if f.m.dlg.edit || len(st.saved) != 0 {
		t.Fatalf("n should discard without saving: %v", st.saved)
	}
	f.keys("n", "e", "x", "esc", "y")
	if len(st.saved) != 1 {
		t.Fatal("y should save")
	}
}

func TestDialogNoNotesOpensEditing(t *testing.T) {
	f, st := dialogFake(t, "")
	f.keys("n")
	if f.m.dlg == nil || !f.m.dlg.edit {
		t.Fatal("no notes should open in edit mode")
	}
	f.keys("esc") // unchanged: back to view mode
	if f.m.dlg.edit {
		t.Fatal("esc without changes leaves edit mode")
	}
	if !strings.Contains(f.m.View().Content, "No notes yet. Press e to write some.") {
		t.Fatal("empty hint missing")
	}
	f.keys("esc")
	if f.m.dlg != nil || len(st.saved) != 0 {
		t.Fatal("esc closes")
	}
}

func TestDialogSwallowsGridKeys(t *testing.T) {
	f, _ := dialogFake(t, "text")
	sel := f.m.sel
	f.keys("n")
	if cmd := f.keys("q"); cmd != nil && cmd() == tea.Quit() {
		t.Fatal("q quit the program")
	}
	if f.m.dlg != nil {
		t.Fatal("q in view mode closes the dialog")
	}
	f.keys("n", "j", "l")
	if f.m.sel != sel {
		t.Fatal("grid moved under the dialog")
	}
	f.keys("esc", "esc")
	if f.m.dlg != nil {
		t.Fatal("esc closes")
	}
}

func TestDialogClickOutside(t *testing.T) {
	f, _ := dialogFake(t, "text")
	f.keys("n")
	f.m.View()
	f.send(tea.MouseClickMsg{Button: tea.MouseLeft, X: 50, Y: 10})
	if f.m.dlg == nil {
		t.Fatal("click inside closed it")
	}
	f.send(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 1})
	if f.m.dlg != nil {
		t.Fatal("click outside should close")
	}
}
