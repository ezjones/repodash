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

func TestUpdateOfferInFooter(t *testing.T) {
	old := core.Version
	core.Version = "v1.0.0"
	t.Cleanup(func() { core.Version = old })
	f := newUpdFake(t)
	footer := func() string {
		ls := strings.Split(f.m.View().Content, "\n")
		return ls[len(ls)-1]
	}
	if strings.Contains(footer(), "U update") {
		t.Fatal("offer shown before any check")
	}
	f.send(updateMsg{tag: "v9.9.9"})
	if l := footer(); !strings.Contains(l, "↑ v9.9.9 · U update") || !strings.Contains(l, "v1.0.0") || strings.Contains(f.m.topBar(), "U update") {
		t.Fatalf("offer not beside the version: %q", l)
	}
	// a click on the chip acts like U
	if cmd := f.send(tea.MouseClickMsg{X: f.m.updX0 + 2, Y: f.m.height - 1, Button: tea.MouseLeft}); cmd != nil {
		cmd()
	}
	if f.applies != 1 {
		t.Fatalf("click on the offer: %d applies", f.applies)
	}
	// narrow: the offer goes, the keys stay
	f.m.msg = ""
	sized(f.fakeTUI, 40, 20)
	if l := footer(); strings.Contains(l, "U update") || !strings.Contains(l, "help") {
		t.Errorf("narrow footer: %q", l)
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

func TestUpdateKeyAsksFresh(t *testing.T) {
	f := newUpdFake(t)
	nows := 0
	f.m.d.CheckUpdateNow = func() (string, error) { nows++; return "v9.9.9", nil }
	cmd := f.keys("U") // nothing offered yet: U checks, skipping the daily cache
	if cmd == nil {
		t.Fatal("U ran no check")
	}
	f.send(cmd())
	if nows != 1 || f.checks != 0 || f.m.newTag != "v9.9.9" {
		t.Fatalf("now %d, daily %d, tag %q", nows, f.checks, f.m.newTag)
	}
}
