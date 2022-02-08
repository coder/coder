package nextrouter

import (
	"bytes"
	"context"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog"
)

// Options for configuring a nextrouter
type Options struct {
	Logger           slog.Logger
	TemplateDataFunc HTMLTemplateHandler
}

// HTMLTemplateHandler is a function that lets the consumer of `nextrouter`
// inject arbitrary template parameters, based on the request. This is useful
// if the Request object is carrying CSRF tokens, session tokens, etc -
// they can be emitted in the page.
type HTMLTemplateHandler func(*http.Request) interface{}

// Handler returns an HTTP handler for serving a next-based static site
// This handler respects NextJS-based routing rules:
// https://nextjs.org/docs/routing/dynamic-routes
//
// 1) If a file is of the form `[org]`, it's a dynamic route for a single-parameter
// 2) If a file is of the form `[[...any]]`, it's a dynamic route for any parameters
func Handler(fileSystem fs.FS, options *Options) (http.Handler, error) {
	if options == nil {
		options = &Options{
			Logger:           slog.Logger{},
			TemplateDataFunc: nil,
		}
	}
	router := chi.NewRouter()

	// Build up a router that matches NextJS routing rules, for HTML files
	err := registerRoutes(router, fileSystem, *options)
	if err != nil {
		return nil, err
	}

	// Fallback to static file server for non-HTML files
	// Non-HTML files don't have special routing rules, so we can just leverage
	// the built-in http.FileServer for it.
	fileHandler := http.FileServer(http.FS(fileSystem))
	router.NotFound(fileHandler.ServeHTTP)

	// Finally, if there is a 404.html available, serve that
	err = register404(fileSystem, router, *options)
	if err != nil {
		// An error may be expected if a 404.html is not present
		options.Logger.Warn(context.Background(), "Unable to find 404.html", slog.Error(err))
	}

	return router, nil
}

// registerRoutes recursively traverses the file-system, building routes
// as appropriate for respecting NextJS dynamic rules.
func registerRoutes(rtr chi.Router, fileSystem fs.FS, options Options) error {
	files, err := fs.ReadDir(fileSystem, ".")
	if err != nil {
		return err
	}

	// Loop through everything in the current directory...
	for _, file := range files {
		name := file.Name()

		// If we're working with a file - just serve it up
		if !file.IsDir() {
			serveFile(rtr, fileSystem, name, options)
			continue
		}

		// ...otherwise, if it's a directory, create a sub-route by
		// recursively calling `buildRouter`
		sub, err := fs.Sub(fileSystem, name)
		if err != nil {
			return err
		}

		// In the special case where the folder is dynamic,
		// like `[org]`, we can convert to a chi-style dynamic route
		// (which uses `{` instead of `[`)
		routeName := name
		if isDynamicRoute(name) {
			routeName = "{dynamic}"
		}

		options.Logger.Debug(context.Background(), "Registering route", slog.F("name", name), slog.F("routeName", routeName))
		rtr.Route("/"+routeName, func(r chi.Router) {
			err := registerRoutes(r, sub, options)
			if err != nil {
				options.Logger.Error(context.Background(), "Error registering route", slog.F("name", routeName), slog.Error(err))
			}
		})
	}

	return nil
}

// serveFile is responsible for serving up HTML files in our next router
// It handles various special cases, like trailing-slashes or handling routes w/o the .html suffix.
func serveFile(router chi.Router, fileSystem fs.FS, fileName string, options Options) {
	// We only handle .html files for now
	ext := filepath.Ext(fileName)
	if ext != ".html" {
		return
	}

	options.Logger.Debug(context.Background(), "Reading file", slog.F("fileName", fileName))

	data, err := fs.ReadFile(fileSystem, fileName)
	if err != nil {
		options.Logger.Error(context.Background(), "Unable to read file", slog.F("fileName", fileName))
		return
	}

	// Create a template from the data - we can inject custom parameters like CSRF here
	tpls, err := template.New(fileName).Parse(string(data))
	if err != nil {
		options.Logger.Error(context.Background(), "Unable to create template for file", slog.F("fileName", fileName))
		return
	}

	handler := func(writer http.ResponseWriter, request *http.Request) {
		var buf bytes.Buffer

		// See if there are any template parameters we need to inject!
		// Things like CSRF tokens, etc...
		//templateData := struct{}{}
		var templateData interface{}
		templateData = nil
		if options.TemplateDataFunc != nil {
			templateData = options.TemplateDataFunc(request)
		}

		options.Logger.Debug(context.Background(), "Applying template parameters", slog.F("fileName", fileName), slog.F("templateData", templateData))
		err := tpls.ExecuteTemplate(&buf, fileName, templateData)

		if err != nil {
			options.Logger.Error(request.Context(), "Error executing template", slog.F("template_parameters", templateData))
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		http.ServeContent(writer, request, fileName, time.Time{}, bytes.NewReader(buf.Bytes()))
	}

	fileNameWithoutExtension := removeFileExtension(fileName)

	// Handle the `[[...any]]` catch-all case
	if isCatchAllRoute(fileNameWithoutExtension) {
		options.Logger.Info(context.Background(), "Registering catch-all route", slog.F("fileName", fileName))
		router.NotFound(handler)
		return
	}

	// Handle the `[org]` dynamic route case
	if isDynamicRoute(fileNameWithoutExtension) {
		options.Logger.Debug(context.Background(), "Registering dynamic route", slog.F("fileName", fileName))
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
		return
	}

	// Otherwise, handling the trailing slash case -
	// for examples, `providers.html` should serve `/providers/`
	router.Get("/"+fileNameWithoutExtension+"/", handler)
}

func register404(fileSystem fs.FS, router chi.Router, options Options) error {
	// Get the file contents
	fileBytes, err := fs.ReadFile(fileSystem, "404.html")
	if err != nil {
		// An error is expected if the file doesn't exist
		return err
	}

	router.NotFound(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, err = writer.Write(fileBytes)
		if err != nil {
			options.Logger.Error(request.Context(), "Unable to write bytes for 404")
			return
		}
	})

	return nil
}

// isDynamicRoute returns true if the file is a NextJS dynamic route, like `[orgs]`
// Returns false if the file is not a dynamic route, or if it is a catch-all route (`[[...any]]`)
// NOTE: The extension should be removed from the file name
func isDynamicRoute(fileWithoutExtension string) bool {
	// Assuming ASCII encoding - `len` in go works on bytes
	byteLen := len(fileWithoutExtension)
	if byteLen < 2 {
		return false
	}

	return fileWithoutExtension[0] == '[' && fileWithoutExtension[1] != '[' && fileWithoutExtension[byteLen-1] == ']'
}

// isCatchAllRoute returns true if the file is a catch-all route, like `[[...any]]`
// Return false otherwise
// NOTE: The extension should be removed from the file name
func isCatchAllRoute(fileWithoutExtension string) bool {
	ret := strings.HasPrefix(fileWithoutExtension, "[[.")
	return ret
}

// removeFileExtension removes the extension from a file
// For example, removeFileExtension("index.html") would return "index"
func removeFileExtension(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}
