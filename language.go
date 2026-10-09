package main

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Main language of a repo, for the cover badge. Counted from tracked file names
// (`git ls-files`), so no file is read. Docs, data and config files are ignored so a
// Go repo with a big README folder is still "Go". Cached, because it is only a badge.

var langByExt = map[string]string{
	".go": "Go", ".py": "Python", ".ts": "TypeScript", ".tsx": "TypeScript", ".js": "JavaScript", ".jsx": "JavaScript",
	".mjs": "JavaScript", ".rs": "Rust", ".swift": "Swift", ".sh": "Shell", ".bash": "Shell", ".zsh": "Shell",
	".cpp": "C++", ".cc": "C++", ".cxx": "C++", ".hpp": "C++", ".c": "C", ".h": "C", ".rb": "Ruby", ".java": "Java",
	".kt": "Kotlin", ".cs": "C#", ".php": "PHP", ".lua": "Lua", ".dart": "Dart", ".zig": "Zig", ".ex": "Elixir",
	".exs": "Elixir", ".hs": "Haskell", ".scala": "Scala", ".vue": "Vue", ".svelte": "Svelte", ".qml": "QML",
}

type langEntry struct {
	lang string
	at   time.Time
}

var (
	langMu    sync.Mutex
	langCache = map[string]langEntry{}
)

func mainLanguage(dir string) string {
	langMu.Lock()
	e, ok := langCache[dir]
	langMu.Unlock()
	if ok && time.Since(e.at) < 10*time.Minute {
		return e.lang
	}
	count := map[string]int{}
	for _, f := range strings.Split(git(dir, "ls-files", "-z"), "\x00") {
		if l := langByExt[strings.ToLower(filepath.Ext(f))]; l != "" {
			count[l]++
		}
	}
	best, n := "", 0
	for l, c := range count {
		if c > n || (c == n && l < best) {
			best, n = l, c
		}
	}
	langMu.Lock()
	langCache[dir] = langEntry{best, time.Now()}
	langMu.Unlock()
	return best
}
