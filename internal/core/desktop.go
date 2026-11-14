package core

// The RepoDash view for the Herdr desktop app: a mod (JavaScript files) copied into the app's
// folder, mods/repodash/. The files are embedded in the binary (desktop.go at the root), so a
// release always carries the matching mod. A stamp beside them records what was installed and
// each file's hash. The one rule: a file nobody can vouch for (changed since it was installed,
// or never RepoDash's) is never replaced or removed unless forced, and then only after a copy
// of it is in a backup folder of its own.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	desktopModName = "repodash"
	desktopStamp   = ".repodash-install.json" // hidden: Herdr does not reload on it
	desktopConfig  = "config.json"            // the mod's settings: ours to fill, the person's to edit
)

// Files the Herdr app writes into every mod folder it loads.
var desktopAppFiles = []string{"jsconfig.json", "gpui-kit.d.ts"}

// ErrNoDesktop means the Herdr desktop app's folder does not exist (the app is not installed, or
// has never been opened).
var ErrNoDesktop = errors.New("Herdr desktop app folder not found")

// DesktopModified lists the files in mods/repodash that were not written by RepoDash, or were
// changed since. The install stops on it unless forced.
type DesktopModified struct {
	Dir   string
	Files []string
}

func (e *DesktopModified) Error() string {
	return fmt.Sprintf("%s has files RepoDash did not write or that were changed since: %s", e.Dir, strings.Join(e.Files, ", "))
}

type desktopStampFile struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"` // path in the mod folder -> sha256
}

// DesktopInstall is what InstallDesktop needs.
type DesktopInstall struct {
	Home       string   // the Herdr app folder (its ~/.config/herdr-app)
	Files      fs.FS    // the mod's files
	Version    string   // this release, recorded in the stamp
	Bin        string   // absolute path of the repodash binary the mod starts; "" keeps the recorded one
	Args       []string // extra arguments for `repodash -web`; nil keeps the recorded ones
	URL        string   // the server's address; "" keeps the recorded one (default http://127.0.0.1:8092)
	Force      bool     // replace changed or foreign files, after copying them to a backup
	BackupRoot string   // where backup folders are made (one new folder per run that needs one)
	Record     string   // file that remembers Home, so later starts can refresh the mod; "" skips it
}

// DesktopResult says what InstallDesktop did.
type DesktopResult struct {
	Dir      string   // the mod folder
	Previous string   // the version it replaced; "" for a fresh install
	Backup   string   // where files were copied before being replaced or removed; "" when none
	Left     []string // uninstall: files left in place (not RepoDash's to remove)
}

func repodashDataDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "repodash")
}

// DesktopRecordPath is the file in RepoDash's data folder that remembers where the mod went.
func DesktopRecordPath() string {
	d := repodashDataDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "desktop.json")
}

// DesktopBackupRoot is where backup folders go: RepoDash's data folder, never under the app's
// mods/ (a folder there with a mod.json would load as another mod).
func DesktopBackupRoot() string { return repodashDataDir() }

// backup copies files into a new folder of their own under root, made on first use, so two runs
// can never write into the same one.
type backup struct {
	root, dir string
}

func (b *backup) add(from, rel string) error {
	if b.dir == "" {
		if b.root == "" || !filepath.IsAbs(b.root) {
			return errors.New("no backup folder for the files to replace")
		}
		if err := os.MkdirAll(b.root, 0o755); err != nil {
			return err
		}
		d, err := os.MkdirTemp(b.root, "desktop-backup-"+time.Now().Format("20060102-150405")+"-")
		if err != nil {
			return err
		}
		b.dir = d
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("backing up %s: %w", rel, err)
	}
	to := filepath.Join(b.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("backing up %s: %w", rel, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("backing up %s: %w", rel, err)
	}
	return f.Close()
}

// desktopRun runs a helper program and returns its trimmed output. Tests replace it.
var desktopRun = func(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// desktopWSL reports whether this is Linux under WSL. Tests replace it.
var desktopWSL = func() bool {
	b, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// DesktopHome finds the Herdr desktop app's folder: $HERDR_APP_HOME, else ~/.config/herdr-app.
// Under WSL the app runs on Windows, so it is the Windows user's folder, seen from Linux
// (/mnt/c/Users/<you>/.config/herdr-app).
func DesktopHome() (string, error) {
	if v := os.Getenv("HERDR_APP_HOME"); v != "" {
		if desktopWSL() && isWindowsPath(v) {
			p, err := desktopRun("/", "wslpath", "-u", v)
			if err != nil || p == "" {
				return "", fmt.Errorf("could not convert %s with wslpath: %v", v, err)
			}
			return p, nil
		}
		return filepath.Abs(v)
	}
	if desktopWSL() {
		// From a Linux folder cmd.exe warns about UNC paths; start it on a Windows drive.
		dir := "/mnt/c"
		if _, err := os.Stat(dir); err != nil {
			dir = "/"
		}
		profile, err := desktopRun(dir, "cmd.exe", "/d", "/c", "echo %USERPROFILE%")
		profile = strings.TrimSpace(profile)
		if err != nil || profile == "" || strings.Contains(profile, "%") {
			return "", fmt.Errorf("could not ask Windows for the user folder (cmd.exe): %v", err)
		}
		home, err := desktopRun("/", "wslpath", "-u", profile)
		if err != nil || home == "" {
			return "", fmt.Errorf("could not convert %s with wslpath: %v", profile, err)
		}
		return filepath.Join(home, ".config", "herdr-app"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "herdr-app"), nil
}

func isWindowsPath(p string) bool {
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') || strings.HasPrefix(p, `\\`)
}

// desktopIgnored are files in the mod folder that are nobody's code: what the app writes into
// every mod folder, config.json (the person's), and hidden ones (.covers, the stamp, temporary
// files). rel is a slash path inside the folder, never ".".
func desktopIgnored(rel string) bool {
	base := path.Base(rel)
	if strings.HasPrefix(base, ".") || strings.Contains(rel, "/.") || rel == desktopConfig {
		return true
	}
	for _, f := range desktopAppFiles {
		if rel == f {
			return true
		}
	}
	return false
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hashFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return hashBytes(b), nil
}

// readDesktopStamp reads the stamp, keeping only entries that are plain paths inside the folder
// and not the person's files: a stamp is only data, and must never point anywhere else.
func readDesktopStamp(dir string) (*desktopStampFile, error) {
	b, err := os.ReadFile(filepath.Join(dir, desktopStamp))
	if err != nil {
		return nil, err
	}
	var s desktopStampFile
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	clean := map[string]string{}
	for rel, h := range s.Files {
		if fs.ValidPath(rel) && rel != "." && !desktopIgnored(rel) {
			clean[rel] = h
		}
	}
	s.Files = clean
	return &s, nil
}

// checkDesktopDir refuses a mod folder that is a symlink or not a folder: what is behind it is
// not RepoDash's to judge.
func checkDesktopDir(dir string) (exists bool, err error) {
	st, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if st.Mode()&fs.ModeSymlink != 0 {
		return true, fmt.Errorf("%s is a symbolic link; RepoDash only installs into a plain folder", dir)
	}
	if !st.IsDir() {
		return true, fmt.Errorf("%s is not a folder", dir)
	}
	return true, nil
}

// desktopChanged lists the files in dir that nobody can vouch for: not in the stamp with the hash
// they have now, and not identical to what this release would write (so a half-finished install
// of the same release is not held against the person). Hidden files and folders, config.json
// and the app's own files are never code and never listed.
func desktopChanged(dir string, incoming map[string]string) ([]string, *desktopStampFile, error) {
	stamp, err := readDesktopStamp(dir)
	if err != nil {
		stamp = nil // missing or unreadable: vouches for nothing
	}
	var changed []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if desktopIgnored(rel) {
			return nil
		}
		got, err := hashFile(p)
		if err != nil {
			return err
		}
		if stamp != nil && stamp.Files[rel] == got {
			return nil
		}
		if incoming != nil && incoming[rel] == got {
			return nil
		}
		changed = append(changed, rel)
		return nil
	})
	sort.Strings(changed)
	return changed, stamp, err
}

// InstallDesktop copies the mod into o.Home/mods/repodash, fills its config.json and writes the
// stamp. Files nobody can vouch for stop it with *DesktopModified, unless o.Force: then they are
// copied to a new backup folder first. config.json is settled before any code file is written;
// helper modules are written before main.js, each through a hidden temporary file, because the
// app reloads the mod on every save.
func InstallDesktop(o DesktopInstall) (DesktopResult, error) {
	res := DesktopResult{}
	if st, err := os.Stat(o.Home); err != nil || !st.IsDir() {
		return res, fmt.Errorf("%w: %s", ErrNoDesktop, o.Home)
	}
	dir := filepath.Join(o.Home, "mods", desktopModName)
	res.Dir = dir
	exists, err := checkDesktopDir(dir)
	if err != nil {
		return res, err
	}

	// What this release writes.
	files := map[string][]byte{}
	incoming := map[string]string{}
	err = fs.WalkDir(o.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || desktopIgnored(p) {
			return nil
		}
		b, err := fs.ReadFile(o.Files, p)
		if err != nil {
			return err
		}
		files[p], incoming[p] = b, hashBytes(b)
		return nil
	})
	if err != nil {
		return res, err
	}
	if len(files) == 0 {
		return res, errors.New("no mod files to install")
	}

	var changed []string
	var stamp *desktopStampFile
	if exists {
		if changed, stamp, err = desktopChanged(dir, incoming); err != nil {
			return res, err
		}
	}
	if stamp != nil {
		res.Previous = stamp.Version
	}
	if len(changed) > 0 && !o.Force {
		return res, &DesktopModified{Dir: dir, Files: changed}
	}
	bk := &backup{root: o.BackupRoot}
	for _, rel := range changed {
		if err := bk.add(filepath.Join(dir, filepath.FromSlash(rel)), rel); err != nil {
			return res, err
		}
	}

	// config.json first: read and merged before anything is written, so a file that can't be
	// read stops the install with the folder as it was.
	cfgPath := filepath.Join(dir, desktopConfig)
	cfg, err := mergeDesktopConfig(cfgPath, o, bk)
	if err != nil {
		return res, err
	}
	res.Backup = bk.dir

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}
	cb, _ := json.MarshalIndent(cfg, "", "  ")
	if err := writeFileAtomic(cfgPath, append(cb, '\n')); err != nil {
		return res, err
	}

	names := make([]string, 0, len(files))
	for rel := range files {
		names = append(names, rel)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "main.js") != (names[j] == "main.js") {
			return names[j] == "main.js"
		}
		return names[i] < names[j]
	})
	for _, rel := range names {
		if err := writeFileAtomic(filepath.Join(dir, filepath.FromSlash(rel)), files[rel]); err != nil {
			return res, err
		}
	}
	// What the previous release installed and this one doesn't ship (its hash still matched, or
	// it was just backed up), and, when forced, the foreign files just backed up.
	gone := map[string]bool{}
	if stamp != nil {
		for rel := range stamp.Files {
			gone[rel] = true
		}
	}
	for _, rel := range changed {
		gone[rel] = true
	}
	for rel := range gone {
		if _, ok := files[rel]; !ok {
			os.Remove(filepath.Join(dir, filepath.FromSlash(rel)))
		}
	}

	sb, _ := json.MarshalIndent(desktopStampFile{Version: o.Version, Files: incoming}, "", "  ")
	if err := writeFileAtomic(filepath.Join(dir, desktopStamp), append(sb, '\n')); err != nil {
		return res, err
	}
	if o.Record != "" {
		rb, _ := json.MarshalIndent(map[string]string{"home": o.Home}, "", "  ")
		if err := os.MkdirAll(filepath.Dir(o.Record), 0o755); err != nil {
			return res, err
		}
		if err := writeFileAtomic(o.Record, append(rb, '\n')); err != nil {
			return res, err
		}
	}
	return res, nil
}

// mergeDesktopConfig returns config.json's keys with bin, args and url from o. A missing file is
// an empty one; one that can't be read is an error; one that isn't a JSON object is copied to
// the backup before it is replaced.
func mergeDesktopConfig(p string, o DesktopInstall, bk *backup) (map[string]any, error) {
	cfg := map[string]any{}
	b, err := os.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("reading %s: %w", p, err)
	default:
		if json.Unmarshal(b, &cfg) != nil || cfg == nil {
			if err := bk.add(p, desktopConfig); err != nil {
				return nil, err
			}
			cfg = map[string]any{}
		}
	}
	if o.Bin != "" {
		cfg["bin"] = o.Bin
	}
	if o.Args != nil {
		cfg["args"] = o.Args
	}
	if o.URL != "" {
		cfg["url"] = o.URL
	}
	return cfg, nil
}

// writeFileAtomic writes through a hidden temporary file of its own in the same folder and
// renames it over.
func writeFileAtomic(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	os.Chmod(tmp, 0o644)
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// UninstallDesktop removes what RepoDash put in mods/repodash: the files its stamp vouches for,
// config.json (copied to a backup first), the cover cache, the stamp and the app's own files.
// Files nobody can vouch for stop it with *DesktopModified unless force, which copies them to a
// backup and removes them too. Anything else (hidden files someone keeps there) stays, with the
// folder, and is listed in Left.
func UninstallDesktop(home string, force bool, backupRoot, record string) (DesktopResult, error) {
	dir := filepath.Join(home, "mods", desktopModName)
	res := DesktopResult{Dir: dir}
	exists, err := checkDesktopDir(dir)
	if err != nil {
		return res, err
	}
	if !exists {
		if record != "" {
			os.Remove(record)
		}
		return res, nil
	}
	changed, stamp, err := desktopChanged(dir, nil)
	if err != nil {
		return res, err
	}
	if len(changed) > 0 && !force {
		return res, &DesktopModified{Dir: dir, Files: changed}
	}
	bk := &backup{root: backupRoot}
	for _, rel := range changed {
		if err := bk.add(filepath.Join(dir, filepath.FromSlash(rel)), rel); err != nil {
			return res, err
		}
	}
	cfg := filepath.Join(dir, desktopConfig)
	if fileExists(cfg) {
		if err := bk.add(cfg, desktopConfig); err != nil {
			return res, err
		}
	}
	res.Backup = bk.dir

	remove := map[string]bool{desktopConfig: true, desktopStamp: true}
	for _, f := range desktopAppFiles {
		remove[f] = true
	}
	if stamp != nil {
		for rel := range stamp.Files {
			remove[rel] = true
		}
	}
	for _, rel := range changed {
		remove[rel] = true
	}
	for rel := range remove {
		os.Remove(filepath.Join(dir, filepath.FromSlash(rel)))
	}
	os.RemoveAll(filepath.Join(dir, ".covers"))
	// Empty folders the mod's files were in, deepest first; then the folder itself, if empty.
	var dirs []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		os.Remove(d) // only succeeds when empty
	}
	if fileExists(dir) {
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(dir, p)
				res.Left = append(res.Left, filepath.ToSlash(rel))
			}
			return nil
		})
		sort.Strings(res.Left)
	}
	if record != "" {
		os.Remove(record)
	}
	return res, nil
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// RefreshDesktop brings an installed mod up to this release: only when one was installed (the
// record exists), its stamp names an older release, and nobody changed its files. It returns
// what it did, for the log; nothing at all for a dev build.
func RefreshDesktop(files fs.FS, version, record string) (string, error) {
	if version == "" || version == "dev" || record == "" {
		return "", nil
	}
	b, err := os.ReadFile(record)
	if err != nil {
		return "", nil
	}
	var r struct {
		Home string `json:"home"`
	}
	if json.Unmarshal(b, &r) != nil || r.Home == "" || !filepath.IsAbs(r.Home) {
		return "", nil
	}
	dir := filepath.Join(r.Home, "mods", desktopModName)
	stamp, err := readDesktopStamp(dir)
	if err != nil || stamp.Version == version || (stamp.Version != "dev" && !NewerVersion(stamp.Version, version)) {
		return "", nil // not installed by us, or this release is not newer
	}
	res, err := InstallDesktop(DesktopInstall{Home: r.Home, Files: files, Version: version, BackupRoot: DesktopBackupRoot()})
	var mod *DesktopModified
	if errors.As(err, &mod) {
		return fmt.Sprintf("desktop view not updated to %s: %v", version, err), nil
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("desktop view updated from %s to %s in %s", stamp.Version, version, res.Dir), nil
}
