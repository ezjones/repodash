package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeContexts(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := ContextFeedDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string, age time.Duration) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-age)
		os.Chtimes(p, at, at)
	}
	usage := `"context_window":{"context_window_size":200000,"current_usage":{"input_tokens":2,"cache_creation_input_tokens":1000,"cache_read_input_tokens":80998}}`
	write("a.json", `{"session_id":"a","cwd":"/src/my-app/sub","workspace":{"project_dir":"/src/my-app"},`+usage+`}`, time.Minute)
	write("b.json", `{"session_id":"b","cwd":"/src/my-app-2",`+usage+`}`, time.Second)
	write("c.json", `{"session_id":"c","cwd":"/src/fresh","context_window":{"context_window_size":1000000,"current_usage":null}}`, 0)
	write("d.json", `{not json`, 0)
	write("old.json", `{"cwd":"/src/old",`+usage+`}`, 8*24*time.Hour)
	write(".x.tmp", `{"cwd":"/src/my-app",`+usage+`}`, 0)

	cs := ClaudeContexts()
	if len(cs) != 3 {
		t.Fatalf("got %d contexts, want 3: %+v", len(cs), cs)
	}
	if u, s := ContextFor(cs, "/src/my-app"); u != 82000 || s != 200000 {
		t.Errorf("my-app = %d/%d, want 82000/200000 (project_dir, not the sibling my-app-2)", u, s)
	}
	if u, _ := ContextFor(cs, "/src/fresh"); u != 0 {
		t.Errorf("a session with no request yet = %d, want 0", u)
	}
	if u, _ := ContextFor(cs, "/src/none"); u != 0 {
		t.Errorf("no session = %d, want 0", u)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.json")); !os.IsNotExist(err) {
		t.Errorf("a file past contextKeep should be removed, stat err = %v", err)
	}

	// a newer session in the same folder wins, and a changed file is read again
	write("e.json", `{"cwd":"/src/my-app","context_window":{"context_window_size":1000000,"current_usage":{"input_tokens":5}}}`, 0)
	if u, s := ContextFor(ClaudeContexts(), "/src/my-app"); u != 5 || s != 1000000 {
		t.Errorf("newest session = %d/%d, want 5/1000000", u, s)
	}
	write("e.json", `{"cwd":"/src/my-app","context_window":{"context_window_size":1000000,"current_usage":{"input_tokens":7000}}}`, 0)
	if u, _ := ContextFor(ClaudeContexts(), "/src/my-app"); u != 7000 {
		t.Errorf("after the file changed = %d, want 7000", u)
	}
}

func TestFormatTokens(t *testing.T) {
	for n, want := range map[int]string{0: "0", 950: "950", 1000: "1k", 82400: "82k", 199600: "200k", 999_499: "999k", 999_500: "1M", 1_000_000: "1M", 1_250_000: "1.2M"} {
		if got := FormatTokens(n); got != want {
			t.Errorf("FormatTokens(%d) = %q, want %q", n, got, want)
		}
	}
}
