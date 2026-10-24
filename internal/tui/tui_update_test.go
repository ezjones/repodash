package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ezjones/repodash/internal/core"
)

type updFake struct {
	*fakeTUI
	checks, applies int
	blocker         string
	applyErr        error
	applied         string
}

func newUpdFake(t *testing.T) *updFake {
	f := &updFake{fakeTUI: newFakeTUI(t, false)}
	f.m.d.CheckUpdate = func() (string, error) { f.checks++; return "v9.9.9", nil }
	f.m.d.UpdateBlocker = func() string { return f.blocker }
	f.m.d.ApplyUpdate = func(tag string) error { f.applies++; f.applied = tag; return f.applyErr }
	sized(f.fakeTUI, 200, 50)
	return f
}

func TestUpdateOfferInTopBar(t *testing.T) {
	f := newUpdFake(t)
	if strings.Contains(f.m.topBar(), "U update") {
		t.Fatal("offer shown before any check")
	}
	f.send(updateMsg{tag: "v9.9.9"})
	if bar := f.m.topBar(); !strings.Contains(bar, "↑ v9.9.9 · U update") {
		t.Errorf("no offer in %q", bar)
	}
	found := false
	for _, z := range f.m.barZones {
		found = found || z.act == "update"
	}
	if !found {
		t.Error("no clickable update zone")
	}
	// narrow: the offer goes, the sort stays
	sized(f.fakeTUI, 40, 20)
	if bar := f.m.topBar(); strings.Contains(bar, "U update") || !strings.Contains(bar, "sort") {
		t.Errorf("narrow bar: %q", bar)
	}
}

func TestUpdateKeyBlocked(t *testing.T) {
	f := newUpdFake(t)
	f.blocker = "built from source: git pull and rebuild"
	f.send(updateMsg{tag: "v9.9.9"})
	if cmd := f.keys("U"); cmd != nil {
		t.Error("blocked update returned a command")
	}
	if f.m.msg != f.blocker || f.applies != 0 {
		t.Errorf("msg %q applies %d", f.m.msg, f.applies)
	}
}

func TestUpdateKeySuccess(t *testing.T) {
	f := newUpdFake(t)
	f.send(updateMsg{tag: "v9.9.9"})
	cmd := f.keys("U")
	if !strings.HasPrefix(f.m.msg, "updating to v9.9.9") || cmd == nil {
		t.Fatalf("msg %q cmd %v", f.m.msg, cmd)
	}
	res := cmd()
	if f.applied != "v9.9.9" {
		t.Errorf("applied %q", f.applied)
	}
	q := f.send(res)
	if !f.m.restart || q == nil {
		t.Fatal("no restart")
	}
	if _, ok := q().(tea.QuitMsg); !ok {
		t.Error("not a quit")
	}
}

func TestUpdateKeyFailure(t *testing.T) {
	f := newUpdFake(t)
	f.applyErr = errors.New("checksum mismatch")
	f.send(updateMsg{tag: "v9.9.9"})
	f.send(f.keys("U")())
	if !strings.Contains(f.m.msg, "update failed: checksum mismatch") || f.m.restart {
		t.Errorf("msg %q restart %v", f.m.msg, f.m.restart)
	}
}

func TestUpdateKeyChecksNow(t *testing.T) {
	f := newUpdFake(t)
	cmd := f.keys("U")
	if cmd == nil {
		t.Fatal("no check")
	}
	f.send(cmd())
	if f.checks != 1 || f.m.newTag != "v9.9.9" {
		t.Errorf("checks %d tag %q", f.checks, f.m.newTag)
	}
	g := newUpdFake(t)
	g.m.d.CheckUpdate = func() (string, error) { return "", nil }
	core.Version = "v1.0.0"
	defer func() { core.Version = "dev" }()
	g.send(g.keys("U")())
	if g.m.msg != "up to date (v1.0.0)" {
		t.Errorf("msg %q", g.m.msg)
	}
}

func TestUpdateSettingOff(t *testing.T) {
	f := newUpdFake(t)
	if f.m.updateTick() == nil {
		t.Fatal("default setting should schedule the check")
	}
	s := tuiSettings()
	old := s.UpdateCheck
	s.UpdateCheck = false
	defer func() { s.UpdateCheck = old }()
	if f.m.updateTick() != nil {
		t.Error("check scheduled although updates.check is off")
	}
	if f.checks != 0 {
		t.Error("check ran")
	}
}

func TestUpdateCheckErrorSilent(t *testing.T) {
	f := newUpdFake(t)
	f.send(updateMsg{err: errors.New("offline")})
	if f.m.msg != "" || f.m.newTag != "" {
		t.Errorf("msg %q", f.m.msg)
	}
}
