package nextrouter

import (
	"bytes"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Handler returns an HTTP handler for serving a next-based static site
// This handler respects NextJS-based routing rules:
// https://nextjs.org/docs/routing/dynamic-routes
//
// 1) If a file is of the form `[org]`, it's a dynamic route for a single-parameter
// 2) If a file is of the form `[[...any]]`, it's a dynamic route for any parameters
func Handler(fileSystem fs.FS) http.Handler {
	router := chi.NewRouter()

	// Build up a router that matches NextJS routing rules, for HTML files
	buildRouter(router, fileSystem)

	// Fallback to static file server for non-HTML files
	// Non-HTML files don't have special routing rules, so we can just leverage
	// the built-in http.FileServer for it.
	fileHandler := http.FileServer(http.FS(fileSystem))
	router.NotFound(fileHandler.ServeHTTP)

	return router
}

func buildRouter(rtr chi.Router, fileSystem fs.FS) {
	files, err := fs.ReadDir(fileSystem, ".")
	if err != nil {
		// TODO(Bryan): Log
		return
	}

	for _, file := range files {
		name := file.Name()

		if file.IsDir() {
			sub, err := fs.Sub(fileSystem, name)
			if err != nil {
				// TODO(Bryan): Log
				continue
			}
			routeName := name

			if isDynamicRoute(name) {
				routeName = "{dynamic}"
			}

			rtr.Route("/"+routeName, func(r chi.Router) {
				buildRouter(r, sub)
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

	data, err := fs.ReadFile(fileSystem, fileName)
	if err != nil {
		// TODO(Bryan): Log here
		return
	}

	// Create a template from the data - we can inject custom parameters like CSRF here
	tpls, err := template.New(fileName).Parse(string(data))
	if err != nil {
		// TODO(Bryan): Log here
		return
	}

	handler := func(w http.ResponseWriter, r *http.Request) {

		var buf bytes.Buffer
		err := tpls.ExecuteTemplate(&buf, fileName, nil)

		// TODO(Bryan): How to handle an error here?
		if err != nil {
			// TODO
			http.Error(w, "500", 500)
			return
		}

		http.ServeContent(w, r, fileName, time.Time{}, bytes.NewReader(buf.Bytes()))
	}

	fileNameWithoutExtension := removeFileExtension(fileName)

	if isCatchAllRoute(fileNameWithoutExtension) {
		router.NotFound(handler)
		return
	}

	// Handle the `[org]` dynamic route case
	if isDynamicRoute(fileNameWithoutExtension) {
		router.Get("/{dynamic}", handler)
		return
	}

	router.Get("/"+fileName, handler)
	router.Get("/"+fileNameWithoutExtension, handler)

	// Special case: '/' should serve index.html
	if fileName == "index.html" {
		router.Get("/", handler)
	} else {
		// Otherwise, handling the trailing slash case -
		// for examples, `providers.html` should serve `/providers/`
		router.Get("/"+fileNameWithoutExtension+"/", handler)
	}
}

func isDynamicRoute(fileName string) bool {
	fileWithoutExtension := removeFileExtension(fileName)

	// Assuming ASCII encoding - `len` in go works on bytes
	byteLen := len(fileWithoutExtension)
	if byteLen < 2 {
		return false
	}

	return fileWithoutExtension[0] == '[' && fileWithoutExtension[1] != '[' && fileWithoutExtension[byteLen-1] == ']'
}

func isCatchAllRoute(fileName string) bool {
	fileWithoutExtension := removeFileExtension(fileName)
	ret := strings.HasPrefix(fileWithoutExtension, "[[.")
	return ret
}

func removeFileExtension(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}
