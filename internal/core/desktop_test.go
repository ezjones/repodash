package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

func modFS(main string) fstest.MapFS {
	return fstest.MapFS{
		"main.js":  {Data: []byte(main)},
		"util.js":  {Data: []byte("export const x = 1;\n")},
		"mod.json": {Data: []byte(`{"place":"none"}`)},
	}
}

func readJSON(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func readStr(p string) string { b, _ := os.ReadFile(p); return string(b) }

// install is InstallDesktop with a fresh backup root per test.
func install(t *testing.T, o DesktopInstall) (DesktopResult, error) {
	t.Helper()
	if o.BackupRoot == "" {
		o.BackupRoot = filepath.Join(t.TempDir(), "data")
	}
	return InstallDesktop(o)
}

func TestInstallDesktopNoApp(t *testing.T) {
	_, err := install(t, DesktopInstall{Home: filepath.Join(t.TempDir(), "missing"), Files: modFS("a")})
	if !errors.Is(err, ErrNoDesktop) {
		t.Fatalf("want ErrNoDesktop, got %v", err)
	}
}

func TestInstallDesktopFreshThenUpgrade(t *testing.T) {
	home := t.TempDir()
	record := filepath.Join(t.TempDir(), "desktop.json")
	res, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1", Bin: "/opt/repodash", Args: []string{"-root", "/code"}, Record: record})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "mods", "repodash")
	if res.Dir != dir || res.Previous != "" || res.Backup != "" {
		t.Fatalf("result %+v", res)
	}
	if readStr(filepath.Join(dir, "main.js")) != "v1" {
		t.Fatal("main.js not written")
	}
	cfg := readJSON(t, filepath.Join(dir, "config.json"))
	if cfg["bin"] != "/opt/repodash" {
		t.Fatalf("config %v", cfg)
	}
	if readJSON(t, record)["home"] != home {
		t.Fatal("record does not name the home")
	}

	// The person's config keys, the cover cache and the app's own files survive; a file the new
	// release no longer ships goes; an untouched install is replaced without -force.
	cfg["machine"] = "box"
	b, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "config.json"), b, 0o644)
	os.MkdirAll(filepath.Join(dir, ".covers"), 0o755)
	os.WriteFile(filepath.Join(dir, ".covers", "a.png"), []byte("png"), 0o644)
	os.WriteFile(filepath.Join(dir, "jsconfig.json"), []byte("{}"), 0o644)
	v2 := modFS("v2")
	delete(v2, "util.js")
	res, err = install(t, DesktopInstall{Home: home, Files: v2, Version: "v2", Bin: "/opt/repodash2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Previous != "v1" || res.Backup != "" {
		t.Fatalf("result %+v", res)
	}
	if readStr(filepath.Join(dir, "main.js")) != "v2" || fileExists(filepath.Join(dir, "util.js")) {
		t.Fatal("main.js not replaced, or util.js not removed")
	}
	if !fileExists(filepath.Join(dir, ".covers", "a.png")) || !fileExists(filepath.Join(dir, "jsconfig.json")) {
		t.Fatal("cache or the app's own file was removed")
	}
	cfg = readJSON(t, filepath.Join(dir, "config.json"))
	if cfg["machine"] != "box" || cfg["bin"] != "/opt/repodash2" {
		t.Fatalf("config %v", cfg)
	}
	if args, _ := cfg["args"].([]any); len(args) != 2 {
		t.Fatalf("args were not kept: %v", cfg["args"])
	}
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temporary file left: %s", e.Name())
		}
	}
}

func TestInstallDesktopKeepsChangedFiles(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "mods", "repodash")
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "main.js"), []byte("mine"), 0o644)
	os.WriteFile(filepath.Join(dir, "extra.js"), []byte("also mine"), 0o644)

	_, err := install(t, DesktopInstall{Home: home, Files: modFS("v2"), Version: "v2"})
	var mod *DesktopModified
	if !errors.As(err, &mod) || strings.Join(mod.Files, ",") != "extra.js,main.js" {
		t.Fatalf("want extra.js and main.js reported, got %v", err)
	}
	if readStr(filepath.Join(dir, "main.js")) != "mine" {
		t.Fatal("a refused install changed main.js")
	}

	root := filepath.Join(t.TempDir(), "data")
	res, err := install(t, DesktopInstall{Home: home, Files: modFS("v2"), Version: "v2", Force: true, BackupRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Backup, root) || readStr(filepath.Join(res.Backup, "main.js")) != "mine" || readStr(filepath.Join(res.Backup, "extra.js")) != "also mine" {
		t.Fatalf("changed files not backed up: %+v", res)
	}
	if readStr(filepath.Join(dir, "main.js")) != "v2" || fileExists(filepath.Join(dir, "extra.js")) {
		t.Fatal("forced install did not replace main.js and clear the backed-up extra.js")
	}
	// Nothing left to block the next plain install.
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v3"), Version: "v3"}); err != nil {
		t.Fatalf("install after -force blocked: %v", err)
	}
}

func TestInstallDesktopForeignFolder(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "mods", "repodash")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "main.js"), []byte("someone else's"), 0o644)
	_, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1"})
	var mod *DesktopModified
	if !errors.As(err, &mod) {
		t.Fatalf("a mod folder RepoDash did not write must stop the install, got %v", err)
	}
	if fileExists(filepath.Join(dir, desktopStamp)) || fileExists(filepath.Join(dir, "config.json")) {
		t.Fatal("something written on a refused install")
	}
}

func TestInstallDesktopSymlinkRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	home := t.TempDir()
	real := filepath.Join(t.TempDir(), "elsewhere")
	os.MkdirAll(real, 0o755)
	os.WriteFile(filepath.Join(real, "main.js"), []byte("mine"), 0o644)
	os.MkdirAll(filepath.Join(home, "mods"), 0o755)
	if err := os.Symlink(real, filepath.Join(home, "mods", "repodash")); err != nil {
		t.Skip(err)
	}
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1", Force: true}); err == nil {
		t.Fatal("a symlinked mod folder must be refused, even with -force")
	}
	if readStr(filepath.Join(real, "main.js")) != "mine" || fileExists(filepath.Join(real, desktopStamp)) {
		t.Fatal("the folder behind the link was written")
	}
	if _, err := UninstallDesktop(home, true, t.TempDir(), ""); err == nil {
		t.Fatal("uninstall through a symlink must be refused")
	}
}

func TestInstallDesktopUnreadableConfig(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads anything")
	}
	home := t.TempDir()
	dir := filepath.Join(home, "mods", "repodash")
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "config.json")
	os.WriteFile(cfg, []byte(`{"machine":"box"}`), 0o644)
	os.Chmod(cfg, 0)
	defer os.Chmod(cfg, 0o644)
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v2"), Version: "v2", Bin: "/b"}); err == nil {
		t.Fatal("an unreadable config.json must stop the install")
	}
	os.Chmod(cfg, 0o644)
	if readStr(cfg) != `{"machine":"box"}` || readStr(filepath.Join(dir, "main.js")) != "v1" {
		t.Fatal("files changed on a refused install")
	}
}

func TestInstallDesktopBadConfigBackedUp(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "mods", "repodash")
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{oops"), 0o644)
	res, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1", Bin: "/b"})
	if err != nil {
		t.Fatal(err)
	}
	if readStr(filepath.Join(res.Backup, "config.json")) != "{oops" {
		t.Fatal("the unreadable config was not backed up")
	}
	if readJSON(t, filepath.Join(dir, "config.json"))["bin"] != "/b" {
		t.Fatal("new config not written")
	}
	// Nothing left behind that blocks the next install.
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v2"), Version: "v2"}); err != nil {
		t.Fatalf("next install blocked: %v", err)
	}
}

func TestInstallDesktopStampCannotReachOutside(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "mods", "repodash")
	keys := filepath.Join(home, "keys.json")
	os.WriteFile(keys, []byte("{}"), 0o644)
	os.MkdirAll(dir, 0o755)
	stamp := `{"version":"v0","files":{"../../keys.json":"x","config.json":"y","/etc/passwd":"z"}}`
	os.WriteFile(filepath.Join(dir, desktopStamp), []byte(stamp), 0o644)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"machine":"box"}`), 0o644)
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(keys) {
		t.Fatal("a stamp entry deleted a file outside the mod folder")
	}
	if readJSON(t, filepath.Join(dir, "config.json"))["machine"] != "box" {
		t.Fatal("a stamp entry removed the person's config")
	}
}

func TestInstallDesktopBackupsNeverShared(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	b1, b2 := &backup{root: root}, &backup{root: root}
	src := filepath.Join(t.TempDir(), "f")
	os.WriteFile(src, []byte("one"), 0o644)
	if err := b1.add(src, "main.js"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(src, []byte("two"), 0o644)
	if err := b2.add(src, "main.js"); err != nil {
		t.Fatal(err)
	}
	if b1.dir == b2.dir || readStr(filepath.Join(b1.dir, "main.js")) != "one" {
		t.Fatal("two runs shared a backup folder")
	}
}

func TestInstallDesktopHalfInstallHeals(t *testing.T) {
	// An install killed after writing some files: those match this release, so the rerun needs
	// no -force.
	home := t.TempDir()
	dir := filepath.Join(home, "mods", "repodash")
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "main.js"), []byte("v2"), 0o644) // as the v2 install left it
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v2"), Version: "v2"}); err != nil {
		t.Fatalf("rerun of the same release blocked: %v", err)
	}
}

func TestRefreshDesktop(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	home := t.TempDir()
	record := filepath.Join(t.TempDir(), "desktop.json")
	dir := filepath.Join(home, "mods", "repodash")
	if msg, _ := RefreshDesktop(modFS("v2"), "v0.2.0", record); msg != "" {
		t.Fatalf("no install, but: %s", msg)
	}
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v0.1.0", Bin: "/b", Record: record}); err != nil {
		t.Fatal(err)
	}
	if msg, _ := RefreshDesktop(modFS("dev"), "dev", record); msg != "" {
		t.Fatalf("a dev build must not touch it: %s", msg)
	}
	if msg, _ := RefreshDesktop(modFS("old"), "v0.0.9", record); msg != "" || readStr(filepath.Join(dir, "main.js")) != "v1" {
		t.Fatalf("an older release must not downgrade it: %s", msg)
	}
	msg, err := RefreshDesktop(modFS("v2"), "v0.2.0", record)
	if err != nil || !strings.Contains(msg, "updated from v0.1.0 to v0.2.0") {
		t.Fatalf("msg %q err %v", msg, err)
	}
	if readJSON(t, filepath.Join(dir, "config.json"))["bin"] != "/b" {
		t.Fatal("refresh lost the recorded binary")
	}
	os.WriteFile(filepath.Join(dir, "main.js"), []byte("mine"), 0o644)
	msg, err = RefreshDesktop(modFS("v3"), "v0.3.0", record)
	if err != nil || !strings.Contains(msg, "not updated") || readStr(filepath.Join(dir, "main.js")) != "mine" {
		t.Fatalf("a changed file must stop the refresh: %q %v", msg, err)
	}
}

func TestUninstallDesktop(t *testing.T) {
	home := t.TempDir()
	record := filepath.Join(t.TempDir(), "desktop.json")
	dir := filepath.Join(home, "mods", "repodash")
	if _, err := install(t, DesktopInstall{Home: home, Files: modFS("v1"), Version: "v1", Bin: "/b", Record: record}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "main.js"), []byte("mine"), 0o644)
	if _, err := UninstallDesktop(home, false, t.TempDir(), record); err == nil {
		t.Fatal("changed files must stop the uninstall")
	}
	// Hidden things someone keeps there are not RepoDash's to remove.
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".covers"), 0o755)
	os.WriteFile(filepath.Join(dir, ".covers", "a.png"), []byte("png"), 0o644)
	root := filepath.Join(t.TempDir(), "data")
	res, err := UninstallDesktop(home, true, root, record)
	if err != nil {
		t.Fatal(err)
	}
	if readStr(filepath.Join(res.Backup, "main.js")) != "mine" || !fileExists(filepath.Join(res.Backup, "config.json")) {
		t.Fatalf("backup incomplete: %+v", res)
	}
	if !fileExists(filepath.Join(dir, ".git", "HEAD")) || len(res.Left) != 1 || res.Left[0] != ".git/HEAD" {
		t.Fatalf(".git must stay and be reported: %+v", res)
	}
	for _, f := range []string{"main.js", "util.js", "mod.json", "config.json", desktopStamp, ".covers"} {
		if fileExists(filepath.Join(dir, f)) {
			t.Fatalf("%s left behind", f)
		}
	}
	if fileExists(record) {
		t.Fatal("record left behind")
	}

	// A clean install goes entirely, folder included.
	home2 := t.TempDir()
	if _, err := install(t, DesktopInstall{Home: home2, Files: modFS("v1"), Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	if res, err := UninstallDesktop(home2, false, root, ""); err != nil || fileExists(filepath.Join(home2, "mods", "repodash")) {
		t.Fatalf("clean uninstall: %+v %v", res, err)
	}
}

func TestDesktopHome(t *testing.T) {
	oldRun, oldWSL := desktopRun, desktopWSL
	defer func() { desktopRun, desktopWSL = oldRun, oldWSL }()
	desktopRun = func(dir, name string, args ...string) (string, error) {
		switch name {
		case "cmd.exe":
			return "C:\\Users\\me\r", nil
		case "wslpath":
			if args[1] == `C:\Users\me` {
				return "/mnt/c/Users/me", nil
			}
			return "/mnt/d/herdr", nil
		}
		return "", errors.New("unexpected")
	}

	t.Setenv("HERDR_APP_HOME", "")
	desktopWSL = func() bool { return false }
	home, _ := os.UserHomeDir()
	if got, _ := DesktopHome(); got != filepath.Join(home, ".config", "herdr-app") {
		t.Fatalf("plain: %s", got)
	}
	desktopWSL = func() bool { return true }
	if got, err := DesktopHome(); err != nil || got != "/mnt/c/Users/me/.config/herdr-app" {
		t.Fatalf("wsl: %s %v", got, err)
	}
	t.Setenv("HERDR_APP_HOME", `D:\herdr`)
	if got, _ := DesktopHome(); got != "/mnt/d/herdr" {
		t.Fatalf("wsl env: %s", got)
	}
	desktopWSL = func() bool { return false }
	t.Setenv("HERDR_APP_HOME", "/tmp/h")
	if got, _ := DesktopHome(); got != "/tmp/h" {
		t.Fatalf("env: %s", got)
	}
	t.Setenv("HERDR_APP_HOME", "rel/h")
	if got, _ := DesktopHome(); !filepath.IsAbs(got) {
		t.Fatalf("a relative HERDR_APP_HOME must come back absolute: %s", got)
	}
}
