package nextrouter

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func serve(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hi"))
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
			rtr.Get("/"+name, serve)
		}
	})

	return rtr, nil
}
