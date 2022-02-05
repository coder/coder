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

// TemplateDataFunc is a function that lets the consumer of `nextrouter`
// inject arbitrary template parameters, based on the request. This is useful
// if the Request object is carrying CSRF tokens, session tokens, etc -
// they can be emitted in the page.
type TemplateDataFunc func(*http.Request) interface{}

// Handler returns an HTTP handler for serving a next-based static site
// This handler respects NextJS-based routing rules:
// https://nextjs.org/docs/routing/dynamic-routes
//
// 1) If a file is of the form `[org]`, it's a dynamic route for a single-parameter
// 2) If a file is of the form `[[...any]]`, it's a dynamic route for any parameters
func Handler(fileSystem fs.FS, templateFunc TemplateDataFunc) http.Handler {
	router := chi.NewRouter()

	// Build up a router that matches NextJS routing rules, for HTML files
	buildRouter(router, fileSystem, templateFunc)

	// Fallback to static file server for non-HTML files
	// Non-HTML files don't have special routing rules, so we can just leverage
	// the built-in http.FileServer for it.
	fileHandler := http.FileServer(http.FS(fileSystem))
	router.NotFound(fileHandler.ServeHTTP)

	return router
}

// buildRouter recursively traverses the file-system, building routes
// as appropriate for respecting NextJS dynamic rules.
func buildRouter(rtr chi.Router, fileSystem fs.FS, templateFunc TemplateDataFunc) {
	files, err := fs.ReadDir(fileSystem, ".")
	if err != nil {
		// TODO(Bryan): Log
		return
	}

	// Loop through everything in the current directory...
	for _, file := range files {
		name := file.Name()

		// ...if it's a directory, create a sub-route by
		// recursively calling `buildRouter`
		if file.IsDir() {
			sub, err := fs.Sub(fileSystem, name)
			if err != nil {
				// TODO(Bryan): Log
				continue
			}

			// In the special case where the folder is dynamic,
			// like `[org]`, we can convert to a chi-style dynamic route
			// (which uses `{` instead of `[`)
			routeName := name
			if isDynamicRoute(name) {
				routeName = "{dynamic}"
			}

			rtr.Route("/"+routeName, func(r chi.Router) {
				buildRouter(r, sub, templateFunc)
			})
		} else {
			// ...otherwise, if it's a file - serve it up!
			serveFile(rtr, fileSystem, name, templateFunc)
		}
	}
}

// serveFile is responsible for serving up HTML files in our next router
// It handles various special cases, like trailing-slashes or handling routes w/o the .html suffix.
func serveFile(router chi.Router, fileSystem fs.FS, fileName string, templateFunc TemplateDataFunc) {
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

	handler := func(writer http.ResponseWriter, request *http.Request) {
		var buf bytes.Buffer

		// See if there are any template parameters we need to inject!
		// Things like CSRF tokens, etc...
		templateData := templateFunc(request)

		err := tpls.ExecuteTemplate(&buf, fileName, templateData)

		// TODO(Bryan): How to handle an error here?
		if err != nil {
			// TODO
			http.Error(writer, "500", 500)
			return
		}

		http.ServeContent(writer, request, fileName, time.Time{}, bytes.NewReader(buf.Bytes()))
	}

	fileNameWithoutExtension := removeFileExtension(fileName)

	// Handle the `[[...any]]` catch-all case
	if isCatchAllRoute(fileNameWithoutExtension) {
		router.NotFound(handler)
		return
	}

	// Handle the `[org]` dynamic route case
	if isDynamicRoute(fileNameWithoutExtension) {
		router.Get("/{dynamic}", handler)
		return
	}

	// Handle the basic file cases
	// Directly accessing a file, ie `/providers.html`
	router.Get("/"+fileName, handler)
	// Accessing a file without an extension, ie `/providers`
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
