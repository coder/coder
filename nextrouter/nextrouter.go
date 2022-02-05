package nextrouter

import (
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Handler returns an HTTP handler for serving a next-based static site
func Handler(fileSystem fs.FS) (http.Handler, error) {
	rtr := chi.NewRouter()
	buildRouter(rtr, fileSystem, "")
	return rtr, nil
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

func serveFile(router chi.Router, fileSystem fs.FS, fileName string) {

	// We only handle .html files for now
	ext := filepath.Ext(fileName)
	if ext != ".html" {
		return
	}

	bytes, err := fs.ReadFile(fileSystem, fileName)
	if err != nil {
		// TODO(Bryan): Log here
		return
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		_, err = w.Write(bytes)

		if err != nil {
			// TODO(BRYAN): Log here
			http.Error(w, http.StatusText(500), 500)
		}
	}

	fileNameWithoutExtension := removeFileExtension(fileName)

	router.Get("/"+fileName, handler)
	router.Get("/"+fileNameWithoutExtension, handler)

	// Special case: '/' should serve index.html
	if fileName == "index.html" {
		router.Get("/", handler)
	}
}

func removeFileExtension(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}
