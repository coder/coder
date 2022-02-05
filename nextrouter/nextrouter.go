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

// Handler returns an HTTP handler for serving a next-based static site
func Handler(fileSystem fs.FS) (http.Handler, error) {
	rtr := chi.NewRouter()

	files, err := fs.ReadDir(fileSystem, ".")
	if err != nil {
		return nil, err
	}

	rtr.Route("/", func(r chi.Router) {
		for _, file := range files {
			name := file.Name()
			rtr.Get("/"+name, serve(fileSystem, name))
		}
	})

	return rtr, nil
}
