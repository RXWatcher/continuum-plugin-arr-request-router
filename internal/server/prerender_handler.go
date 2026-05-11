package server

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// htmlTagRE matches the opening <html> tag (with or without existing
// attributes), case-insensitive.
//
// CONSTRAINT: This regex replaces the WHOLE <html ...> tag, so any existing
// attributes (e.g. lang="en") are lost in the rewrite. The SPA scaffold's
// index.html deliberately avoids attributes other than the injected data-theme,
// so this is acceptable. If index.html is ever updated to include lang= or
// class= attributes, this handler must be updated to preserve them (e.g. by
// using a capture-and-append approach instead of a full replacement).
var htmlTagRE = regexp.MustCompile(`(?i)<html(\s[^>]*)?>`)

// headTagRE matches the opening <head> tag (with or without attributes),
// case-insensitive. Used to inject <base href> so Vite's relative asset
// paths resolve to the plugin root regardless of the document URL.
var headTagRE = regexp.MustCompile(`(?i)<head(\s[^>]*)?>`)

// computeBaseHref returns the relative path needed in <base href="..."> so
// that relative asset URLs in the SPA's index.html (e.g. ./assets/foo.js,
// emitted by Vite) resolve to the plugin's root regardless of the document
// URL. The algorithm counts slashes in the request path directory to figure
// out how many "../" segments are needed to climb back to /.
//
//	/admin                   → "./"
//	/admin/                  → "../"
//	/admin/registry          → "../"
//	/admin/registry/         → "../../"
//	/admin/registry/123/edit → "../../../"
func computeBaseHref(reqPath string) string {
	lastSlash := strings.LastIndex(reqPath, "/")
	if lastSlash < 0 {
		return "./"
	}
	dir := reqPath[:lastSlash+1]
	depth := strings.Count(dir, "/") - 1
	if depth <= 0 {
		return "./"
	}
	return strings.Repeat("../", depth)
}

// handleSPA serves index.html with `data-theme="<theme>"` injected onto
// the <html> element. The theme is read from the X-Continuum-Theme header
// (preferred — continuum's plugin proxy injects it) or from the ?theme=…
// query string (fallback — the sidebar appends it on direct navigation).
// Falls back to "default" if neither is present.
//
// Cache-Control: no-store — the theme varies per request.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if s.deps.WebFS == nil {
		http.Error(w, "spa not embedded", http.StatusInternalServerError)
		return
	}
	f, err := s.deps.WebFS.Open("/index.html")
	if err != nil {
		// Some http.FileSystem implementations expect "index.html" without slash.
		f, err = s.deps.WebFS.Open("index.html")
	}
	if err != nil {
		http.Error(w, "spa missing", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "spa read", http.StatusInternalServerError)
		return
	}

	theme := r.Header.Get("X-Continuum-Theme")
	if theme == "" {
		theme = r.URL.Query().Get("theme")
	}
	if theme == "" {
		theme = "default"
	}
	safeTheme := strings.ReplaceAll(theme, `"`, "&quot;")

	out := htmlTagRE.ReplaceAll(raw, []byte(`<html data-theme="`+safeTheme+`">`))

	baseHref := computeBaseHref(r.URL.Path)
	out = headTagRE.ReplaceAllFunc(out, func(m []byte) []byte {
		// Preserve the original opening <head ...> and append <base href=...>
		return append(append([]byte{}, m...), []byte(`<base href="`+baseHref+`">`)...)
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(bytes.TrimSpace(out))
}
