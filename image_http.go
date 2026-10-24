package main

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func serveImage(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path, ok := CoverFile(root, r.PathValue("name"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		fi, _ := f.Stat()
		w.Header().Set("Content-Type", ImageType(strings.ToLower(filepath.Ext(path))))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// If someone opens an SVG directly, it still may not run script.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, path, fi.ModTime(), f)
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
		ext := ImageExt(ct)
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
		if err := SaveCustomImage(name, ext, b); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteImage(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if _, ok := validRepo(root, name); !ok {
			http.Error(w, "not a repo", http.StatusNotFound)
			return
		}
		RemoveCustomImage(name)
		w.WriteHeader(http.StatusNoContent)
	}
}
