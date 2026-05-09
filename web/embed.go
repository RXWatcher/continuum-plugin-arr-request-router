// Package web embeds the built SPA. The embed sits in web/ so it can reach
// the sibling dist/ directory; //go:embed is constrained to the package
// directory and its descendants.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var dist embed.FS

// FS returns the SPA file system rooted at dist/.
func FS() http.FileSystem {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("web: " + err.Error())
	}
	return http.FS(sub)
}

// SPAHandler returns an http.Handler that serves the embedded SPA. For paths
// that don't match a real file, it serves index.html so client-side routing
// works.
func SPAHandler() http.Handler {
	fileSrv := http.FileServer(FS())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := FS().Open(r.URL.Path)
		if err != nil {
			r.URL.Path = "/"
		} else {
			_ = f.Close()
		}
		fileSrv.ServeHTTP(w, r)
	})
}
