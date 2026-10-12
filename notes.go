package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The info panel: a repo's README (read from the repo, never written) and your own
// notes (stored under notesDir, never inside the repo).

const (
	maxReadme = 256 << 10
	maxNotes  = 256 << 10
)

var (
	notesDir    string // set in main
	notesMu     sync.Mutex
	readmeNames = []string{"README.md", "readme.md", "Readme.md", "README.markdown", "README", "README.txt"}
)

func notesPath(name string) string { return filepath.Join(notesDir, name+".md") }

func hasNotes(name string) bool {
	fi, err := os.Stat(notesPath(name))
	return err == nil && fi.Size() > 0
}

// readmeFile returns the repo's README text. A README that is a symlink leading
// outside the repo is ignored, like cover images.
func readmeFile(repoDir string) (string, string, bool) {
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
	_, _, ok := readmeFile(repoDir)
	return ok
}

// infoGet answers {readme, readme_name, notes} for one repo.
func infoGet(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir, ok := validRepo(root, r.PathValue("name"))
		if !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		out := map[string]any{"readme": nil, "readme_name": "", "notes": ""}
		if n, text, ok := readmeFile(dir); ok {
			out["readme"], out["readme_name"] = text, n
		}
		notesMu.Lock()
		if b, err := os.ReadFile(notesPath(r.PathValue("name"))); err == nil {
			out["notes"] = string(b)
		}
		notesMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(out)
	}
}

// notesPut saves {"text": "..."}; empty text removes the file. JSON content type
// only, for the same cross-site reason as the layout PUT.
func notesPut(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		name := r.PathValue("name")
		if _, ok := validRepo(root, name); !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxNotes+4096)).Decode(&req); err != nil || len(req.Text) > maxNotes {
			http.Error(w, "invalid or too large", http.StatusBadRequest)
			return
		}
		notesMu.Lock()
		defer notesMu.Unlock()
		p := notesPath(name)
		if strings.TrimSpace(req.Text) == "" {
			os.Remove(p)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := os.MkdirAll(notesDir, 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmp := p + ".tmp"
		if err := os.WriteFile(tmp, []byte(req.Text), 0o644); err == nil {
			err = os.Rename(tmp, p)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
