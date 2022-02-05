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

func buildRouter(rtr chi.Router, fileSystem fs.FS, path string) {
	files, err := fs.ReadDir(fileSystem, ".")
	if err != nil {
		// TODO(Bryan): Log
		return
	}

	rtr.Route("/", func(r chi.Router) {
		for _, file := range files {
			name := file.Name()

			if file.IsDir() {
				sub, err := fs.Sub(fileSystem, name)
				if err != nil {
					// TODO(Bryan): Log
					continue
				}
				buildRouter(r, sub, path+"/"+name)
			} else {
				rtr.Get("/"+name, serve(fileSystem, name))
			}
		}
	})
}

// Handler returns an HTTP handler for serving a next-based static site
func Handler(fileSystem fs.FS) (http.Handler, error) {
	rtr := chi.NewRouter()
	buildRouter(rtr, fileSystem, "")
	return rtr, nil
}
