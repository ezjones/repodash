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

// readNameList returns the names in the {"<key>": [names]} file at path. A missing or unreadable
// file means none.
func readNameList(path, key string) map[string]bool {
	m := map[string]bool{}
	if path == "" {
		return m
	}
	var f map[string][]string
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &f)
	}
	for _, n := range f[key] {
		m[n] = true
	}
	return m
}

// setNameListed adds or removes one name in the {"<key>": [names]} file at path. It takes the
// wanted state, not a toggle, and reads the file first, so the TUI and the web server writing in
// turn never undo each other. A file that is not valid JSON is left alone (it may be mid-edit by
// hand) and the change is not saved. Other keys in the file are kept.
func setNameListed(mu *sync.Mutex, path, key, name string, on bool) error {
	if path == "" || NotesPath(name) == "" {
		return errors.New("cannot save " + name + " in " + key)
	}
	mu.Lock()
	defer mu.Unlock()
	f := map[string]any{}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &f); err != nil {
			return fmt.Errorf("%s is not valid JSON, so this was not saved: %v", path, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	var names []string
	if l, ok := f[key].([]any); ok {
		for _, v := range l {
			if s, ok := v.(string); ok {
				names = append(names, s)
			}
		}
	}
	i := slices.Index(names, name)
	switch {
	case on && i < 0:
		names = append(names, name)
	case !on && i >= 0:
		names = slices.Delete(names, i, i+1)
	default:
		return nil
	}
	if names == nil {
		names = []string{}
	}
	f[key] = names
	b, err = json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	t, err := os.CreateTemp(dir, key+"-*.json")
	if err != nil {
		return err
	}
	_, err = t.Write(append(b, '\n'))
	if cerr := t.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(t.Name(), path)
	}
	if err != nil {
		os.Remove(t.Name())
	}
	return err
}

// ReadPins returns the pinned repo names. A missing or unreadable file means none.
func ReadPins() map[string]bool { return readNameList(PinsPath(), "pinned") }

// SetPinned pins or unpins one repo (see setNameListed for the guarantees).
func SetPinned(name string, on bool) error {
	return setNameListed(&pinsMu, PinsPath(), "pinned", name, on)
}
