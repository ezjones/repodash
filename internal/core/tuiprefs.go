package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// TUIPrefs are choices made in the terminal UI that it remembers between runs. They live in the
// data folder, not in repodash.json, which people edit by hand.
type TUIPrefs struct {
	HideHints bool `json:"hideHints,omitempty"` // the footer's key hints are off (H)
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
