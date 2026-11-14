package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ezjones/repodash/internal/core"
)

// desktopFiles is the RepoDash view for the Herdr desktop app (herdr-app/mods/repodash), which
// `repodash -install-desktop` copies into the app's folder. Embedding it means a release always
// installs the mod that matches its own server.
//
// Listed by pattern so files a local Herdr writes into a working copy (jsconfig.json, *.d.ts,
// config.json, .covers) never end up in the binary.
//
//go:embed herdr-app/mods/repodash/*.js herdr-app/mods/repodash/mod.json herdr-app/mods/repodash/*.svg
var desktopFiles embed.FS

func desktopMod() fs.FS {
	sub, err := fs.Sub(desktopFiles, "herdr-app/mods/repodash")
	if err != nil {
		panic(err)
	}
	return sub
}

// installDesktop is -install-desktop: copy the mod in and say how to open it. No Herdr app is
// not an error (install.sh runs this everywhere); files someone changed are.
func installDesktop(force bool, args []string, url string) int {
	home, err := core.DesktopHome()
	if err != nil {
		fmt.Fprintln(os.Stderr, "repodash: finding the Herdr desktop app:", err)
		return 1
	}
	bin, err := os.Executable()
	if err == nil {
		if real, err := filepath.EvalSymlinks(bin); err == nil {
			bin = real
		}
	}
	res, err := core.InstallDesktop(core.DesktopInstall{
		Home: home, Files: desktopMod(), Version: buildVersion, Bin: bin, Args: args, URL: url,
		Force: force, BackupRoot: core.DesktopBackupRoot(), Record: core.DesktopRecordPath(),
	})
	var mod *core.DesktopModified
	switch {
	case errors.Is(err, core.ErrNoDesktop):
		fmt.Printf("No Herdr desktop app here (%s), so no desktop view; add it later with: repodash -install-desktop\n", home)
		return 0
	case errors.As(err, &mod):
		fmt.Fprintf(os.Stderr, "repodash: %v\n", err)
		fmt.Fprintln(os.Stderr, "Nothing was changed. To replace them (each is copied to RepoDash's data folder first), run: repodash -install-desktop -force")
		return 1
	case err != nil:
		fmt.Fprintln(os.Stderr, "repodash: installing the desktop view:", err)
		return 1
	}
	if res.Previous != "" {
		fmt.Printf("Updated the RepoDash view for the Herdr desktop app (%s -> %s): %s\n", res.Previous, buildVersion, res.Dir)
	} else {
		fmt.Printf("Installed the RepoDash view for the Herdr desktop app: %s\n", res.Dir)
	}
	if res.Backup != "" {
		fmt.Printf("The files it replaced were copied to %s first.\n", res.Backup)
	}
	fmt.Println("Open it in Herdr with the command palette (Ctrl+Shift+P): mod.open, then repodash.")
	fmt.Println("For a key, add to keys.json in the same folder (pick a key you don't use):")
	fmt.Println(`  "global": { "ctrl-shift-r": "mod.open:repodash" }`)
	fmt.Println("It shows the repos of the RepoDash server and starts `" + filepath.Base(bin) + " -web` itself when none is running.")
	return 0
}

func uninstallDesktop(force bool) int {
	home, err := core.DesktopHome()
	if err != nil {
		fmt.Fprintln(os.Stderr, "repodash: finding the Herdr desktop app:", err)
		return 1
	}
	res, err := core.UninstallDesktop(home, force, core.DesktopBackupRoot(), core.DesktopRecordPath())
	var mod *core.DesktopModified
	switch {
	case errors.As(err, &mod):
		fmt.Fprintf(os.Stderr, "repodash: %v\n", err)
		fmt.Fprintln(os.Stderr, "Nothing was removed. To remove them anyway (a copy goes to RepoDash's data folder first), run: repodash -uninstall-desktop -force")
		return 1
	case err != nil:
		fmt.Fprintln(os.Stderr, "repodash: removing the desktop view:", err)
		return 1
	}
	fmt.Printf("Removed the RepoDash view from the Herdr desktop app (%s).\n", res.Dir)
	if res.Backup != "" {
		fmt.Printf("Its settings and any changed files were copied to %s.\n", res.Backup)
	}
	if len(res.Left) > 0 {
		fmt.Printf("Left in place (not RepoDash's): %s\n", strings.Join(res.Left, ", "))
	}
	fmt.Println("If you added a key for mod.open:repodash in keys.json, remove that line too.")
	return 0
}

// refreshDesktop updates an installed desktop view to this release, in the background of -web.
func refreshDesktop() {
	msg, err := core.RefreshDesktop(desktopMod(), buildVersion, core.DesktopRecordPath())
	if err != nil {
		log.Printf("desktop view: %v", err)
	} else if msg != "" {
		log.Print(msg)
	}
}

// desktopServerArgs are the -web arguments the desktop view should start the server with: the
// ones given to -install-desktop that change what it serves, made absolute. Nil (keep what was
// recorded) when none was given, so a plain re-run, as install.sh does, changes nothing.
func desktopServerArgs(given map[string]bool, root, cfg, addr string) (args []string, url string) {
	if !given["root"] && !given["config"] && !given["addr"] {
		return nil, ""
	}
	args = []string{}
	if given["root"] {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		args = append(args, "-root", root)
	}
	if given["config"] && cfg != "" {
		if abs, err := filepath.Abs(cfg); err == nil {
			cfg = abs
		}
		args = append(args, "-config", cfg)
	}
	if given["addr"] {
		args = append(args, "-addr", addr)
		url = "http://" + clientAddr(addr)
	}
	return args, url
}

// clientAddr is where a client on this machine reaches a server listening on addr: a missing
// or "any" host means this machine.
func clientAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
