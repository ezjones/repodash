package main

import (
	"fmt"
	"github.com/ezjones/repodash/internal/core"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// webURL is the address a browser on this machine uses for a -addr value.
func webURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// webUp reports whether a RepoDash server answers at u.
func webUp(u string) bool {
	c := http.Client{Timeout: time.Second}
	r, err := c.Get(u + "/api/config")
	if err != nil {
		return false
	}
	r.Body.Close()
	return r.StatusCode == http.StatusOK
}

// openWeb opens the web dashboard in the browser. When no server answers on addr it first
// starts one in the background (repodash -web plus args), detached so it outlives the TUI,
// logging to logPath.
func openWeb(addr string, args []string, logPath string) (string, error) {
	u := webURL(addr)
	if !webUp(u) {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		os.MkdirAll(filepath.Dir(logPath), 0o755)
		lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return "", err
		}
		cmd := exec.Command(exe, append([]string{"-web", "-no-open", "-addr", addr}, args...)...)
		cmd.Stdout, cmd.Stderr = lf, lf
		detach(cmd)
		err = cmd.Start()
		lf.Close()
		if err != nil {
			return "", err
		}
		go cmd.Wait()
		for i := 0; i < 30 && !webUp(u); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		if !webUp(u) {
			return "", fmt.Errorf("web server did not start on %s (see %s)", addr, logPath)
		}
	}
	core.OpenBrowser(u)
	return u, nil
}
