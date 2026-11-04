package core

import (
	"path/filepath"
	"sync"
)

// Hidden repos are left out of the terminal UI and the page unless "show hidden" is on. Nothing
// on disk changes except this list, hidden.json in the data folder beside pins.json, shared by
// both. Hidden repos are still scanned. (The settings key repos.<name>.hidden is different: it
// drops the repo from the scan.)

var hiddenMu sync.Mutex

// HiddenPath is the hidden-repos file, or "" before the data folders are set.
func HiddenPath() string {
	if notesDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(notesDir), "hidden.json")
}

// ReadHidden returns the hidden repo names. A missing or unreadable file means none.
func ReadHidden() map[string]bool { return readNameList(HiddenPath(), "hidden") }

// SetHidden hides or unhides one repo (see setNameListed for the guarantees).
func SetHidden(name string, on bool) error {
	return setNameListed(&hiddenMu, HiddenPath(), "hidden", name, on)
}
