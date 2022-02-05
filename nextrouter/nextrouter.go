package nextrouter

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func serveFile(router chi.Router, fileSystem fs.FS, fileName string) {

	handler := func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Requesting file: " + fileName)
		bytes, err := fs.ReadFile(fileSystem, fileName)

		if err != nil {
			http.Error(w, http.StatusText(404), 404)
		}
		_, err = w.Write(bytes)

		if err != nil {
			http.Error(w, http.StatusText(404), 404)
		}
	}

	router.Get("/"+fileName, handler)
}

func buildRouter(rtr chi.Router, fileSystem fs.FS, name string) {
	files, err := fs.ReadDir(fileSystem, ".")
	if err != nil {
		// TODO(Bryan): Log
		return
	}

	fmt.Println("Recursing: " + name)
	for _, file := range files {
		name := file.Name()

		if file.IsDir() {
			sub, err := fs.Sub(fileSystem, name)
			if err != nil {
				// TODO(Bryan): Log
				continue
			}
			rtr.Route("/"+name, func(r chi.Router) {
				buildRouter(r, sub, name)
			})
		} else {
			serveFile(rtr, fileSystem, name)

		}
	}
}

// Handler returns an HTTP handler for serving a next-based static site
func Handler(fileSystem fs.FS) (http.Handler, error) {
	rtr := chi.NewRouter()
	buildRouter(rtr, fileSystem, "")
	return rtr, nil
}
