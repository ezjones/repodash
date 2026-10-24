package core

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Cover images. Each card gets one, picked in this order:
//
//  1. an image you added yourself (stored under imageDir, never inside the repo)
//  2. an "image" path for the repo in repodash.json
//  3. a logo/icon/cover/screenshot-style file in the repo root (names from the settings)
//  4. the same names inside assets/, docs/, public/ and similar folders (dirs from the settings)
//  5. the first local image the README references (unless covers.readme is false)
//
// Remote README images (badges, hosted screenshots) are skipped on purpose:
// the dashboard never makes the browser fetch from the internet.

const MaxImage = 4 << 20

var (
	imageDir   string // set in main
	imgMime    = map[string]string{".png": "image/png", ".svg": "image/svg+xml", ".webp": "image/webp", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".avif": "image/avif"}
	imgExtPref = []string{".png", ".svg", ".webp", ".jpg", ".jpeg", ".gif", ".avif"}
	readmeRef  = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?([^)\s>]+)|(?i:<img\b[^>]*?\bsrc\s*=\s*["']([^"']+)["'])`)
)

type cover struct {
	path string // file on disk
	src  string // custom | root | dir | readme
	fit  string // cover | contain
	ver  string // cache-busting version
}

func (c cover) url(name string) string { return "/img/" + url.PathEscape(name) + "?v=" + c.ver }

// ValidRepo maps a request's repo name to its directory, refusing anything that
// is not a plain git checkout directly under root.
func ValidRepo(root, name string) (string, bool) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", false
	}
	dir := filepath.Join(root, name)
	fi, err := os.Stat(filepath.Join(dir, ".git"))
	return dir, err == nil && fi.IsDir()
}

// repoImage accepts a file only if, after resolving symlinks, it still lives
// inside the repo (a README must not be able to point at ~/.ssh) and is small enough.
func repoImage(repoDir, p string) (string, os.FileInfo, bool) {
	if _, ok := imgMime[strings.ToLower(filepath.Ext(p))]; !ok {
		return "", nil, false
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", nil, false
	}
	base, err := filepath.EvalSymlinks(repoDir)
	if err != nil || !strings.HasPrefix(real, base+string(filepath.Separator)) {
		return "", nil, false
	}
	fi, err := os.Stat(real)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > MaxImage || fi.Size() == 0 {
		return "", nil, false
	}
	return real, fi, true
}

func version(fi os.FileInfo) string {
	return strconv.FormatInt(fi.ModTime().UnixMilli(), 36) + strconv.FormatInt(fi.Size(), 36)
}

func customImage(name string) (string, os.FileInfo, bool) {
	if name != filepath.Base(name) {
		return "", nil, false
	}
	for _, ext := range imgExtPref {
		p := filepath.Join(imageDir, name+ext)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p, fi, true
		}
	}
	return "", nil, false
}

// pickByName finds the best logo-ish file in one directory.
func pickByName(repoDir, dir string, coverNames []string) (string, os.FileInfo, string, bool) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, "", false
	}
	have := map[string]string{}
	var names []string // ReadDir is sorted, so prefix matches resolve the same way every scan
	for _, e := range ents {
		have[strings.ToLower(e.Name())] = e.Name()
		names = append(names, strings.ToLower(e.Name()))
	}
	try := func(real, n string) (string, os.FileInfo, string, bool) {
		if p, fi, ok := repoImage(repoDir, filepath.Join(dir, real)); ok {
			fit := "cover"
			if n == "logo" || n == "icon" {
				fit = "contain"
			}
			return p, fi, fit, true
		}
		return "", nil, "", false
	}
	for _, n := range coverNames {
		for _, ext := range imgExtPref { // logo.png beats logo.jpg
			if real, ok := have[n+ext]; ok {
				if p, fi, fit, ok := try(real, n); ok {
					return p, fi, fit, true
				}
			}
		}
		for _, l := range names { // then logo-navbar.png, screenshot_1.png, ...
			if len(l) > len(n) && strings.HasPrefix(l, n) && strings.ContainsRune("-_ .", rune(l[len(n)])) {
				if p, fi, fit, ok := try(have[l], n); ok {
					return p, fi, fit, true
				}
			}
		}
	}
	return "", nil, "", false
}

type readmeInfo struct {
	mod  int64
	refs []string
}

var readmeCache sync.Map // path -> readmeInfo

func readmeRefs(repoDir string) []string {
	ents, err := os.ReadDir(repoDir)
	if err != nil {
		return nil
	}
	for _, e := range ents {
		l := strings.ToLower(e.Name())
		if l != "readme.md" && l != "readme.markdown" {
			continue
		}
		p := filepath.Join(repoDir, e.Name())
		fi, err := os.Stat(p)
		if err != nil || fi.Size() > 512<<10 {
			return nil
		}
		if v, ok := readmeCache.Load(p); ok && v.(readmeInfo).mod == fi.ModTime().UnixNano() {
			return v.(readmeInfo).refs
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		var refs []string
		for _, m := range readmeRef.FindAllStringSubmatch(string(b), -1) {
			if r := m[1] + m[2]; r != "" {
				refs = append(refs, r)
			}
		}
		readmeCache.Store(p, readmeInfo{fi.ModTime().UnixNano(), refs})
		return refs
	}
	return nil
}

func findCover(root, name string) (cover, bool) {
	// Validate first: name comes straight from the URL (/img/{name}) and %2f decodes
	// to "/", so an unchecked name would walk out of imageDir.
	dir, ok := ValidRepo(root, name)
	if !ok {
		return cover{}, false
	}
	if p, fi, ok := customImage(name); ok {
		return cover{p, "custom", "cover", version(fi)}, true
	}
	s := cfg.get()
	if rel := s.repos[name].Image; rel != "" { // set in repodash.json
		if p, fi, ok := repoImage(dir, filepath.Join(dir, filepath.FromSlash(rel))); ok {
			return cover{p, "config", "cover", version(fi)}, true
		}
	}
	if p, fi, fit, ok := pickByName(dir, dir, s.coverNames); ok {
		return cover{p, "root", fit, version(fi)}, true
	}
	for _, d := range s.coverDirs {
		if p, fi, fit, ok := pickByName(dir, filepath.Join(dir, d), s.coverNames); ok {
			return cover{p, "dir", fit, version(fi)}, true
		}
	}
	if !s.coverReadme {
		return cover{}, false
	}
	for _, ref := range readmeRefs(dir) {
		l := strings.ToLower(ref)
		if strings.HasPrefix(l, "http:") || strings.HasPrefix(l, "https:") || strings.HasPrefix(l, "//") || strings.HasPrefix(l, "data:") {
			continue
		}
		ref, _, _ = strings.Cut(ref, "#")
		ref, _, _ = strings.Cut(ref, "?")
		if u, err := url.PathUnescape(ref); err == nil {
			ref = u
		}
		if p, fi, ok := repoImage(dir, filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(ref, "/")))); ok {
			return cover{p, "readme", "cover", version(fi)}, true
		}
	}
	return cover{}, false
}

func removeCustom(name string) {
	for _, ext := range imgExtPref {
		os.Remove(filepath.Join(imageDir, name+ext))
	}
}

// CoverFile returns the cover image file for a repo, as served at /img/<repo>.
func CoverFile(root, name string) (string, bool) {
	c, ok := findCover(root, name)
	return c.path, ok
}

// ImageType is the MIME type for an image file extension (".png"), or "" for an unknown one.
func ImageType(ext string) string { return imgMime[ext] }

// ImageExt is the file extension (".png") for an image MIME type, or "" when it is not one we store.
func ImageExt(contentType string) string {
	ext := ""
	for e, m := range imgMime {
		if m == contentType && e != ".jpeg" {
			ext = e
		}
	}
	return ext
}

// SaveCustomImage stores b as the repo's own cover, replacing any earlier one.
func SaveCustomImage(name, ext string, b []byte) error {
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return err
	}
	removeCustom(name)
	return os.WriteFile(filepath.Join(imageDir, name+ext), b, 0o644)
}

// RemoveCustomImage deletes the repo's own cover, if it has one.
func RemoveCustomImage(name string) { removeCustom(name) }
