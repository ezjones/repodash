package main

import "testing"

func TestDesktopServerArgs(t *testing.T) {
	// A plain re-run (install.sh) keeps what was recorded: nil args, no url.
	if args, url := desktopServerArgs(map[string]bool{}, ".", "", "127.0.0.1:8092"); args != nil || url != "" {
		t.Fatalf("plain run: %v %q", args, url)
	}
	args, url := desktopServerArgs(map[string]bool{"root": true, "addr": true}, "/srv/code", "", ":9000")
	if len(args) != 4 || args[1] != "/srv/code" || args[3] != ":9000" || url != "http://127.0.0.1:9000" {
		t.Fatalf("got %v %q", args, url)
	}
	for in, want := range map[string]string{"0.0.0.0:8092": "127.0.0.1:8092", "[::]:8092": "127.0.0.1:8092", "myhost:8092": "myhost:8092"} {
		if got := clientAddr(in); got != want {
			t.Errorf("clientAddr(%q) = %q, want %q", in, got, want)
		}
	}
}
