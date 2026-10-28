package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Pinned repos sort first in every view. The list lives in pins.json in the data folder, beside
// the notes folder, and is shared by the terminal UI and the web page.

var pinsMu sync.Mutex

// PinsPath is the pins file, or "" before the data folders are set.
func PinsPath() string {
	if notesDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(notesDir), "pins.json")
}

type pinsFile struct {
	Pinned []string `json:"pinned"`
}

// ReadPins returns the pinned repo names. A missing or unreadable file means none.
func ReadPins() map[string]bool {
	m := map[string]bool{}
	p := PinsPath()
	if p == "" {
		return m
	}
	var f pinsFile
	if b, err := os.ReadFile(p); err == nil {
		json.Unmarshal(b, &f)
	}
	for _, n := range f.Pinned {
		m[n] = true
	}
	return m
}

// SetPinned pins or unpins one repo. It takes the wanted state, not a toggle, and reads the file
// first, so the TUI and the web server writing in turn never undo each other. A file that is not
// valid JSON is left alone (it may be mid-edit by hand) and the change is not saved.
func SetPinned(name string, on bool) error {
	p := PinsPath()
	if p == "" || NotesPath(name) == "" {
		return errors.New("cannot pin " + name)
	}
	pinsMu.Lock()
	defer pinsMu.Unlock()
	var f pinsFile
	b, err := os.ReadFile(p)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &f); err != nil {
			return fmt.Errorf("%s is not valid JSON, so this was not saved: %v", p, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	i := slices.Index(f.Pinned, name)
	switch {
	case on && i < 0:
		f.Pinned = append(f.Pinned, name)
	case !on && i >= 0:
		f.Pinned = slices.Delete(f.Pinned, i, i+1)
	default:
		return nil
	}
	if f.Pinned == nil {
		f.Pinned = []string{}
	}
	b, err = json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	t, err := os.CreateTemp(dir, "pins-*.json")
	if err != nil {
		return err
	}
	_, err = t.Write(append(b, '\n'))
	if cerr := t.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(t.Name(), p)
	}
	if err != nil {
		os.Remove(t.Name())
	}
	return err
}
