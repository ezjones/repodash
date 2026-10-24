package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// TUIPrefs are choices made in the terminal UI that it remembers between runs. They live in the
// data folder, not in repodash.json, which people edit by hand.
type TUIPrefs struct {
	HideHints    bool              `json:"hideHints,omitempty"`    // the footer's key hints are off (H)
	Theme        string            `json:"theme,omitempty"`        // card theme; "" is the default (colourful)
	StatusDot    bool              `json:"statusDot,omitempty"`    // status as a coloured dot, no words
	HideLanguage bool              `json:"hideLanguage,omitempty"` // no language badge on the cards
	Colors       map[string]string `json:"colors,omitempty"`       // repo name -> its own band colour, #rrggbb
}

func TUIPrefsPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "repodash", "tui.json")
}

// ReadTUIPrefs returns the saved choices, or the zero value when there are none.
func ReadTUIPrefs(path string) TUIPrefs {
	var p TUIPrefs
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &p)
	}
	return p
}

// UpdateTUIPrefs applies change to the choices on disk and writes them back. Reading first means
// two running TUIs only overwrite what each one changed. A file that is not valid JSON is left
// alone (it may be mid-edit by hand) and the change is not saved.
func UpdateTUIPrefs(path string, change func(*TUIPrefs)) error {
	var p TUIPrefs
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &p); err != nil {
			return fmt.Errorf("%s is not valid JSON, so this was not saved: %v", path, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	change(&p)
	return WriteTUIPrefs(path, p)
}

func WriteTUIPrefs(path string, p TUIPrefs) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "tui-*.json")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}
