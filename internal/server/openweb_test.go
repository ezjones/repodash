package server

import "testing"

func TestWebURL(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:8092": "http://127.0.0.1:8092",
		":8092":          "http://127.0.0.1:8092",
		"0.0.0.0:9000":   "http://127.0.0.1:9000",
		"[::]:8092":      "http://127.0.0.1:8092",
		"[::1]:8092":     "http://[::1]:8092",
	} {
		if got := webURL(in); got != want {
			t.Errorf("webURL(%q) = %q, want %q", in, got, want)
		}
	}
}
