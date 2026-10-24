// RepoDash: a live dashboard of every git checkout under ~/gitrepos.
//
//	repodash                       terminal UI (w opens the web page); without a terminal, the server
//	repodash -web                  serve the dashboard on 127.0.0.1:8092
//	repodash -addr :8092           reachable from other devices (tailnet, LAN)
//	repodash -json                 scan once, print JSON, exit (the future CLI starts here)
//	repodash -check                validate repodash.json, print problems, exit 1 if any
//	repodash -print-config         print every setting with its default value
//
// Settings live in repodash.json next to the binary (so they are part of the repo)
// and apply live. See AGENTS.md.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ezjones/repodash/internal/core"
	"github.com/ezjones/repodash/internal/tui"
	"log"
	"os"
	"path/filepath"
)

// version is set at release build time with -ldflags "-X main.buildVersion=...".
var buildVersion = "dev"

func main() {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	root := flag.String("root", core.DefaultRoot(cwd, home), "directory holding the repos (default: the current folder if it holds git repos, else ~/gitrepos)")
	addr := flag.String("addr", "127.0.0.1:8092", "listen address")
	allowHost := flag.String("allow-host", "", "extra host names the page may be opened by, comma separated (needed when -addr is reachable by name, e.g. a tailnet name)")
	cfgPath := flag.String("config", "", "settings file (default: repodash.json next to the binary, or $REPODASH_CONFIG)")
	once := flag.Bool("json", false, "scan once, print JSON, exit")
	check := flag.Bool("check", false, "validate the settings file, print problems, exit 1 if there are any")
	tuiFlag := flag.Bool("tui", false, "open the terminal UI (the default when run from a terminal)")
	web := flag.Bool("web", false, "serve the web dashboard (the default when not run from a terminal)")
	herdrOpen := flag.Bool("herdr-open", false, "herdr plugin action: focus the RepoDash tab, or open one")
	noOpen := flag.Bool("no-open", false, "do not open the page in a browser on start")
	showVersion := flag.Bool("version", false, "print the version and exit")
	printCfg := flag.Bool("print-config", false, "print the default settings and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("repodash", buildVersion)
		return
	}

	if *printCfg {
		os.Stdout.Write(core.DefaultConfigJSON())
		return
	}
	if *herdrOpen {
		if err := core.HerdrOpenTUI(); err != nil {
			log.Fatal(err)
		}
		return
	}
	p := settingsPath(*cfgPath)
	if !*check && !*once {
		core.EnsureConfigFile(p)
	}
	core.LoadConfig(p)

	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		dataDir = filepath.Join(home, ".local", "share")
	}
	core.SetDataDirs(filepath.Join(dataDir, "repodash", "images"), filepath.Join(dataDir, "repodash", "notes"))

	if *check {
		s := core.Config()
		if len(s.Warnings) == 0 {
			fmt.Printf("%s: ok\n", core.ConfigPath())
			return
		}
		fmt.Printf("%s: %d problem(s)\n", core.ConfigPath(), len(s.Warnings))
		for _, w := range s.Warnings {
			fmt.Println("  -", w)
		}
		os.Exit(1)
	}
	if *once {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(core.ScanAll(*root))
		return
	}

	// From a terminal, plain `repodash` is the TUI; server flags, -web, or no terminal
	// (nohup, systemd) mean the web server.
	serverFlag := *web
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "addr", "allow-host", "no-open":
			serverFlag = true
		}
	})
	if *tuiFlag || (!serverFlag && core.Interactive() && core.StdinTerminal()) {
		args := []string{"-root", *root, "-config", core.ConfigPath()}
		if *allowHost != "" {
			args = append(args, "-allow-host", *allowHost)
		}
		openWebPage := func() (string, error) {
			return openWeb(*addr, args, filepath.Join(dataDir, "repodash", "repodash.log"))
		}
		if err := tui.Run(*root, openWebPage); err != nil {
			log.Fatal(err)
		}
		return
	}
	log.Fatal(serve(serveOptions{
		Root:       *root,
		Addr:       *addr,
		AllowHost:  *allowHost,
		LayoutPath: filepath.Join(dataDir, "repodash", "layout.json"),
		NoOpen:     *noOpen,
	}))
}

// settingsPath picks the settings file: -config, then $REPODASH_CONFIG, then
// repodash.json beside the executable when that is a source checkout (go.mod
// next to it) or already has one, so a `go build` in the repo keeps its
// settings versioned with the code. An installed binary (bin dir) instead uses
// $XDG_CONFIG_HOME/repodash/repodash.json, so no config lands in a bin folder.
func settingsPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv("REPODASH_CONFIG"); v != "" {
		return v
	}
	beside := "repodash.json"
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		dir := filepath.Dir(exe)
		beside = filepath.Join(dir, "repodash.json")
		for _, marker := range []string{"go.mod", "repodash.json"} {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return beside
			}
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "repodash", "repodash.json")
	}
	return beside
}
