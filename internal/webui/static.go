package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed frontend/dist
var frontendDist embed.FS

// spaHandler serves the built frontend, falling back to index.html for any
// path that doesn't match a real static asset (client-side routing).
func spaHandler() http.Handler {
	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		panic("webui: frontend/dist not embedded correctly: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(distFS, r.URL.Path[1:]); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
