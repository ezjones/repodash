package core

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The info panel: a repo's README (read from the repo, never written) and your own
// notes (stored under notesDir, never inside the repo).

const (
	maxReadme = 256 << 10
	MaxNotes  = 256 << 10
)

var (
	notesDir    string // set in main
	notesMu     sync.Mutex
	readmeNames = []string{"README.md", "readme.md", "Readme.md", "README.markdown", "README", "README.txt"}
)

func notesPath(name string) string { return filepath.Join(notesDir, name+".md") }

// NotesPath is where the repo's notes file lives (it may not exist yet), or "" when name is not a
// plain folder name: the same names the page accepts, nothing with a separator or dots only.
func NotesPath(name string) string {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return ""
	}
	return notesPath(name)
}

func hasNotes(name string) bool {
	fi, err := os.Stat(notesPath(name))
	return err == nil && fi.Size() > 0
}

// ReadmeFile returns the repo's README text. A README that is a symlink leading
// outside the repo is ignored, like cover images.
func ReadmeFile(repoDir string) (string, string, bool) {
	realRepo, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		return "", "", false
	}
	for _, n := range readmeNames {
		p, err := filepath.EvalSymlinks(filepath.Join(repoDir, n))
		if err != nil || !strings.HasPrefix(p, realRepo+string(filepath.Separator)) {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		buf := make([]byte, maxReadme)
		k, _ := f.Read(buf)
		f.Close()
		return n, string(buf[:k]), true
	}
	return "", "", false
}

func hasReadme(repoDir string) bool {
	_, _, ok := ReadmeFile(repoDir)
	return ok
}

// NotesExcerpt returns the first lines non-empty lines of the repo's notes, or nil.
func NotesExcerpt(name string, lines int) []string {
	b, err := os.ReadFile(notesPath(name))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
			if len(out) == lines {
				break
			}
		}
	}
	return out
}

// ReadNotes returns the repo's notes text, and false when there is no notes file.
func ReadNotes(name string) (string, bool) {
	notesMu.Lock()
	defer notesMu.Unlock()
	b, err := os.ReadFile(notesPath(name))
	if err != nil {
		return "", false
	}
	return string(b), true
}

// SaveNotes stores the repo's notes; blank text removes the file. Only a failure to create the
// notes folder is reported: a failed write is ignored, as the page has always done.
func SaveNotes(name, text string) error {
	notesMu.Lock()
	defer notesMu.Unlock()
	p := notesPath(name)
	if strings.TrimSpace(text) == "" {
		os.Remove(p)
		return nil
	}
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err == nil {
		os.Rename(tmp, p)
	}
	return nil
}
