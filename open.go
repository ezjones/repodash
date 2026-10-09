package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// openBrowser opens url in the user's default browser, best effort. On WSL it
// hands the URL to Windows so it opens in the Windows browser. Errors are
// ignored: the URL is already printed in the startup log line.
func openBrowser(url string) {
	var cmds [][]string
	switch runtime.GOOS {
	case "darwin":
		cmds = [][]string{{"open", url}}
	default:
		if b, err := os.ReadFile("/proc/version"); err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft") {
			cmds = append(cmds, []string{"wslview", url}, []string{"cmd.exe", "/c", "start", "", url})
		}
		cmds = append(cmds, []string{"xdg-open", url})
	}
	for _, c := range cmds {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		if exec.Command(c[0], c[1:]...).Start() == nil {
			return
		}
	}
}

// interactive reports whether stdout is a terminal, so a service, a pipe or a
// redirect to /dev/null never tries to open a browser.
func interactive() bool {
	fi, err := os.Stdout.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return true
}
