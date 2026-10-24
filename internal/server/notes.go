package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ezjones/repodash/internal/core"
)

// infoGet answers {readme, readme_name, notes} for one repo.
func infoGet(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir, ok := core.ValidRepo(root, r.PathValue("name"))
		if !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		out := map[string]any{"readme": nil, "readme_name": "", "notes": ""}
		if n, text, ok := core.ReadmeFile(dir); ok {
			out["readme"], out["readme_name"] = text, n
		}
		if text, ok := core.ReadNotes(r.PathValue("name")); ok {
			out["notes"] = text
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(out)
	}
}

// commitsGet answers {commits:[{hash,at,subject}], web?}: the repo's 5 newest commits.
func commitsGet(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir, ok := core.ValidRepo(root, r.PathValue("name"))
		if !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		type c struct {
			Hash    string `json:"hash"`
			At      int64  `json:"at"`
			Subject string `json:"subject"`
		}
		out := []c{}
		for _, l := range core.RecentCommits(dir, 5) {
			out = append(out, c{l.Hash, l.At, l.Subject})
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		resp := map[string]any{"commits": out}
		if slug := core.GitHubRepo(core.Git(dir, "config", "--get", "remote.origin.url")); slug != "" {
			resp["web"] = "https://github.com/" + slug
		}
		json.NewEncoder(w).Encode(resp)
	}
}

// commitGet answers one commit's details (CommitInfo) for the inspector. Read-only.
func commitGet(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir, ok := core.ValidRepo(root, r.PathValue("name"))
		if !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		if !core.ValidHash(r.PathValue("hash")) {
			http.Error(w, "bad hash", http.StatusBadRequest)
			return
		}
		ci, err := core.CommitDetail(dir, r.PathValue("hash"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(ci)
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
		if _, ok := core.ValidRepo(root, name); !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, core.MaxNotes+4096)).Decode(&req); err != nil || len(req.Text) > core.MaxNotes {
			http.Error(w, "invalid or too large", http.StatusBadRequest)
			return
		}
		if err := core.SaveNotes(name, req.Text); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
