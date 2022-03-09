//go:build !slim
// +build !slim

package site

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"
	"github.com/unrolled/secure"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// The `embed` package ignores recursively including directories
// that prefix with `_`. Wildcarding nested is janky, but seems to
// work quite well for edge-cases.
//go:embed out
//go:embed out/bin/*
var site embed.FS

// HTMLTemplateHandler is a function that defines how `htmlState` is populated
type HTMLTemplateHandler func(*http.Request) HtmlState

// DefaultHandler returns an HTTP handler for serving the static site,
// based on the `embed.FS` compiled into the binary.
func DefaultHandler(logger slog.Logger) http.Handler {
	filesystem, err := fs.Sub(site, "out")
	if err != nil {
		// This can't happen... Go would throw a compilation error.
		panic(err)
	}

	templateFunc := func(r *http.Request) HtmlState {
		return HtmlState{
			// CSP nonce for the given request (if there is one present)
			CSPNonce: secure.CSPNonce(r.Context()),
			// CSRF token for the given request
			CSRFToken: nosurf.Token(r),
		}
	}

	return Handler(filesystem, logger, templateFunc)
}

// Handler returns an HTTP handler for serving the static site.
// This takes a filesystem as a parameter.
func Handler(filesystem fs.FS, logger slog.Logger, templateFunc HTMLTemplateHandler) http.Handler {
	router := chi.NewRouter()

	staticFileHandler, err := serveFiles(filesystem, logger)
	if err != nil {
		panic(err)
	}

	router.NotFound(staticFileHandler)

	return secureHeaders(router)
}

func serveFiles(fileSystem fs.FS, logger slog.Logger) (http.HandlerFunc, error) {
	// htmlFileToTemplate is a map of html files -> template
	// We need to use templates in order to inject parameters from `HtmlState`
	// (like CSRF token and CSP nonce)
	htmlFileToTemplate := map[string]*template.Template{}

	// nonHtmlFileToTemplate is a map of files -> byte contents
	// This is used for any non-HTML file
	nonHtmlFileToTemplate := map[string][]byte{}

	// fallbackHtmlTemplate is used as the 'default' template if
	// the path requested doesn't match anything on the file systme.
	var fallbackHtmlTemplate *template.Template

	files, err := fs.ReadDir(fileSystem, ".")
	if err != nil {
		return nil, err
	}

	// Loop through everything in the current directory...
	for _, file := range files {
		name := file.Name()
		normalizedName := strings.ToLower(name)

		// If we're working with a file - just serve it up
		if !file.IsDir() {
			fileBytes, err := fs.ReadFile(fileSystem, normalizedName)

			if err != nil {
				logger.Warn(context.Background(), "Unable to load file", slog.F("fileName", normalizedName))
				continue
			}

			isHtml := isHtmlFile(normalizedName)
			if isHtml {
				// For HTML files, we need to parse and store the template.
				// If its index.html, we need to keep a reference to it as well.
				template, err := template.New("").Parse(string(fileBytes))
				if err != nil {
					logger.Warn(context.Background(), "Unable to parse html template", slog.F("fileName", normalizedName))
					continue
				}

				htmlFileToTemplate[normalizedName] = template
				// If this is the index page, use it as the fallback template
				if strings.HasPrefix(normalizedName, "index.") {
					fallbackHtmlTemplate = template
				}
			} else {
				// Non HTML files are easy - just cache the bytes
				nonHtmlFileToTemplate[normalizedName] = fileBytes
			}

			continue
		}

		// If we reached here, there was something on the file system (most likely a directory)
		// that we were unable to handle in the current code - so log a warning.
		logger.Warn(context.Background(), "Serving from nested directories is not implemented", slog.F("name", name))
	}

	// If we don't have a default template, then there's not much to do!
	if fallbackHtmlTemplate == nil {
		return nil, xerrors.Errorf("No index.html found")
	}

	serveFunc := func(writer http.ResponseWriter, request *http.Request) {
		fileName := filepath.Base(request.URL.Path)
		normalizedFileName := strings.ToLower(fileName)

		if normalizedFileName == "/" {
			normalizedFileName = "index.html"
		}

		// First, let's look at our non-HTML files to see if this matches
		fileBytes, ok := nonHtmlFileToTemplate[normalizedFileName]
		if ok {
			// All our assets - JavaScript, CSS, images - should be cached.
			// For cases like JavaScript, we rely on a cache-busting strategy whenever
			// there is a new version (this is handled in our webpack config).
			writer.Header().Add("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeContent(writer, request, normalizedFileName, time.Time{}, bytes.NewReader(fileBytes))
			return
		}

		var buf bytes.Buffer
		// TODO: Fix this
		templateData := HtmlState{
			CSRFToken: "TODO",
			CSPNonce:  "TODO",
		}

		// Next, lets try and load from our HTML templates
		template, ok := htmlFileToTemplate[normalizedFileName]
		if ok {
			logger.Debug(context.Background(), "Applying template parameters", slog.F("fileName", normalizedFileName), slog.F("templateData", templateData))
			err := template.ExecuteTemplate(&buf, "", templateData)

			if err != nil {
				logger.Error(request.Context(), "Error executing template", slog.F("templateData", templateData))
				http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			http.ServeContent(writer, request, normalizedFileName, time.Time{}, bytes.NewReader(buf.Bytes()))
			return
		}

		// Finally... the path didn't match any file that we had cached.
		// This is expected, because any nested path is going to hit this case.
		// For that, we'll serve the fallback
		logger.Debug(context.Background(), "Applying template parameters", slog.F("fileName", normalizedFileName), slog.F("templateData", templateData))
		err := fallbackHtmlTemplate.ExecuteTemplate(&buf, "", templateData)

		if err != nil {
			logger.Error(request.Context(), "Error executing template", slog.F("templateData", templateData))
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		http.ServeContent(writer, request, normalizedFileName, time.Time{}, bytes.NewReader(buf.Bytes()))

	}

	return serveFunc, nil
}

func isHtmlFile(fileName string) bool {
	return strings.HasSuffix(fileName, ".html") || strings.HasSuffix(fileName, ".htm")
}

type HtmlState struct {
	CSPNonce  string
	CSRFToken string
}

// cspDirectives is a map of all csp fetch directives to their values.
// Each directive is a set of values that is joined by a space (' ').
// All directives are semi-colon separated as a single string for the csp header.
type cspDirectives map[cspFetchDirective][]string

// cspFetchDirective is the list of all constant fetch directives that
// can be used/appended to.
type cspFetchDirective string

const (
	cspDirectiveDefaultSrc  = "default-src"
	cspDirectiveConnectSrc  = "connect-src"
	cspDirectiveChildSrc    = "child-src"
	cspDirectiveScriptSrc   = "script-src"
	cspDirectiveFontSrc     = "font-src"
	cspDirectiveStyleSrc    = "style-src"
	cspDirectiveObjectSrc   = "object-src"
	cspDirectiveManifestSrc = "manifest-src"
	cspDirectiveFrameSrc    = "frame-src"
	cspDirectiveImgSrc      = "img-src"
	cspDirectiveReportURI   = "report-uri"
	cspDirectiveFormAction  = "form-action"
	cspDirectiveMediaSrc    = "media-src"
	cspFrameAncestors       = "frame-ancestors"
)

// secureHeaders is only needed for statically served files. We do not need this for api endpoints.
// It adds various headers to enforce browser security features.
func secureHeaders(next http.Handler) http.Handler {
	// Content-Security-Policy disables loading certain content types and can prevent XSS injections.
	// This site helps eval your policy for syntax and other common issues: https://csp-evaluator.withgoogle.com/
	// If we ever want to render something like a PDF, we need to adjust "object-src"
	//
	//	The list of CSP options: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Security-Policy/default-src
	cspSrcs := cspDirectives{
		// All omitted fetch csp srcs default to this.
		cspDirectiveDefaultSrc: {"'self'"},
		cspDirectiveConnectSrc: {"'self' ws: wss:"},
		cspDirectiveChildSrc:   {"'self'"},
		cspDirectiveScriptSrc:  {"'self'"},
		cspDirectiveFontSrc:    {"'self'"},
		cspDirectiveStyleSrc:   {"'self' 'unsafe-inline'"},
		// object-src is needed to support code-server
		cspDirectiveObjectSrc: {"'self'"},
		// blob: for loading the pwa manifest for code-server
		cspDirectiveManifestSrc: {"'self' blob:"},
		cspDirectiveFrameSrc:    {"'self'"},
		// data: for loading base64 encoded icons for generic applications.
		cspDirectiveImgSrc:     {"'self' https://cdn.coder.com data:"},
		cspDirectiveFormAction: {"'self'"},
		cspDirectiveMediaSrc:   {"'self'"},
		// Report all violations back to the server to log
		cspDirectiveReportURI: {"/api/private/csp/reports"},
		cspFrameAncestors:     {"'none'"},

		// Only scripts can manipulate the dom. This prevents someone from
		// naming themselves something like '<svg onload="alert(/cross-site-scripting/)" />'.
		// TODO: @emyrk we need to make FE changes to enable this. We get 'TrustedHTML' and 'TrustedURL' errors
		//		that require FE changes to work.
		// "require-trusted-types-for" : []string{"'script'"},
	}

	var csp strings.Builder
	for src, vals := range cspSrcs {
		_, _ = fmt.Fprintf(&csp, "%s %s; ", src, strings.Join(vals, " "))
	}

	// Permissions-Policy can be used to disabled various browser features that we do not use.
	// This can prevent an embedded iframe from accessing these features.
	// If we support arbitrary iframes such as generic applications, we might need to add permissions
	// based on the app here.
	permissions := strings.Join([]string{
		// =() means it is disabled
		"accelerometer=()",
		"autoplay=()",
		"battery=()",
		"camera=()",
		"document-domain=()",
		"geolocation=()",
		"gyroscope=()",
		"magnetometer=()",
		"microphone=()",
		"midi=()",
		"payment=()",
		"usb=()",
		"vr=()",
		"screen-wake-lock=()",
		"xr-spatial-tracking=()",
	}, ", ")

	return secure.New(secure.Options{
		// Set to ContentSecurityPolicyReportOnly for testing, as all errors are printed to the console log
		// but are not enforced.
		ContentSecurityPolicy: csp.String(),

		PermissionsPolicy: permissions,

		// Prevent the browser from sending Referer header with requests
		ReferrerPolicy: "no-referrer",
	}).Handler(next)
}
