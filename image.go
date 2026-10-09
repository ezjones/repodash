package main

import (
	"bytes"
	"io"
	"mime"
	"net/http"
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

const maxImage = 4 << 20

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

// validRepo maps a request's repo name to its directory, refusing anything that
// is not a plain git checkout directly under root.
func validRepo(root, name string) (string, bool) {
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
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxImage || fi.Size() == 0 {
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
	dir, ok := validRepo(root, name)
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

func serveImage(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, ok := findCover(root, r.PathValue("name"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(c.path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		fi, _ := f.Stat()
		w.Header().Set("Content-Type", imgMime[strings.ToLower(filepath.Ext(c.path))])
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// If someone opens an SVG directly, it still may not run script.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, c.path, fi.ModTime(), f)
	}
}

func putImage(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if _, ok := validRepo(root, name); !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		ext := ""
		for e, m := range imgMime {
			if m == ct && e != ".jpeg" {
				ext = e
			}
		}
		if ext == "" {
			http.Error(w, "send the image with an image/* content type (png, jpeg, webp, gif, svg, avif)", http.StatusUnsupportedMediaType)
			return
		}
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImage))
		if err != nil || len(b) == 0 {
			http.Error(w, "image is empty or larger than 4 MB", http.StatusRequestEntityTooLarge)
			return
		}
		// Trust the bytes, not the header.
		if ext == ".svg" {
			if !bytes.Contains(bytes.ToLower(b), []byte("<svg")) {
				http.Error(w, "not an SVG", http.StatusBadRequest)
				return
			}
		} else if !strings.HasPrefix(http.DetectContentType(b), "image/") && ext != ".avif" {
			http.Error(w, "not an image", http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(imageDir, 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		removeCustom(name)
		if err := os.WriteFile(filepath.Join(imageDir, name+ext), b, 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func removeCustom(name string) {
	for _, ext := range imgExtPref {
		os.Remove(filepath.Join(imageDir, name+ext))
	}
}

func deleteImage(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if _, ok := validRepo(root, name); !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		removeCustom(name)
		w.WriteHeader(http.StatusNoContent)
	}
}
