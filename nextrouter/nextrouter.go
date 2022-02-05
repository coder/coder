package nextrouter

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func serve(fileSystem fs.FS, filePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		bytes, err := fs.ReadFile(fileSystem, filePath)

		if err != nil {
			http.Error(w, http.StatusText(404), 404)
		}
		_, err = w.Write(bytes)

		if err != nil {
			http.Error(w, http.StatusText(404), 404)
		}
	}
}

func buildRouter(parentFileSystem fs.FS, path string) (chi.Router, error) {
	fileSystem, err := fs.Sub(parentFileSystem, path)
	if err != nil {
		return nil, err
	}
	files, err := fs.ReadDir(fileSystem, ".")
	rtr := chi.NewRouter()
	rtr.Route("/", func(r chi.Router) {
		for _, file := range files {
			name := file.Name()
			rtr.Get("/"+name, serve(fileSystem, name))
		}
	})
	return rtr, nil
}

// Handler returns an HTTP handler for serving a next-based static site
func Handler(fileSystem fs.FS) (http.Handler, error) {

	router, err := buildRouter(fileSystem, ".")
	if err != nil {
		return nil, err
	}
	return router, nil
}
