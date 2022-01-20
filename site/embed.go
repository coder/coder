package site

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"text/template" // html/template escapes some nonces
	"time"

	"github.com/justinas/nosurf"
	"github.com/unrolled/secure"
	"golang.org/x/xerrors"
)

// The `embed` package ignores recursively including directories
// that prefix with `_`. Wildcarding nested is janky, but seems to
// work quite well for edge-cases.
//go:embed out/_next/*/*/*/*
//go:embed out/_next/*/*/*
//go:embed out
var site embed.FS

// Handler returns an HTTP handler for serving the static site.
func Handler() http.Handler {
	filesystem, err := fs.Sub(site, "out")
	if err != nil {
		// This can't happen... Go would throw a compilation error.
		panic(err)
	}

	// html files are handled by a text/template. Non-html files
	// are served by the default file server.
	files, err := htmlFiles(filesystem)
	if err != nil {
		panic(xerrors.Errorf("Failed to return handler for static files. Html files failed to load: %w", err))
	}

	return secureHeaders(&handler{
		fs:        filesystem,
		htmlFiles: files,
		h:         http.FileServer(http.FS(filesystem)), // All other non-html static files
	})
}

type handler struct {
	fs fs.FS
	// htmlFiles is the text/template for all *.html files.
	// This is needed to support Content Security Policy headers.
	// Due to material UI, we are forced to use a nonce to allow inline
	// scripts, and that nonce is passed through a template.
	// We only do this for html files to reduce the amount of in memory caching
	// of duplicate files as `fs`.
	htmlFiles *htmlTemplates
	h         http.Handler
}

// filePath returns the filepath of the requested file.
func (*handler) filePath(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimPrefix(path.Clean(p), "/")
}

func (h *handler) exists(filePath string) bool {
	f, err := h.fs.Open(filePath)
	if err == nil {
		_ = f.Close()
	}
	return err == nil
}

type htmlState struct {
	CSP  cspState
	CSRF csrfState
}

type cspState struct {
	Nonce string
}

type csrfState struct {
	Token string
}

func (h *handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// reqFile is the static file requested
	reqFile := h.filePath(r.URL.Path)
	state := htmlState{
		// Nonce is the CSP nonce for the given request (if there is one present)
		CSP: cspState{Nonce: secure.CSPNonce(r.Context())},
		// Token is the CSRF token for the given request
		CSRF: csrfState{Token: nosurf.Token(r)},
	}

	// First check if it's a file we have in our templates
	if h.serveHTML(rw, r, reqFile, state) {
		return
	}

	// If the original file path exists we serve it.
	if h.exists(reqFile) {
		h.h.ServeHTTP(rw, r)
		return
	}

	// Serve the file assuming it's an html file
	// This matches paths like `/app/terminal.html`
	r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
	r.URL.Path += ".html"

	reqFile = h.filePath(r.URL.Path)
	// All html files should be served by the htmlFile templates
	if h.serveHTML(rw, r, reqFile, state) {
		return
	}

	// If we don't have the file... we should redirect to `/`
	// for our single-page-app.
	r.URL.Path = "/"
	if h.serveHTML(rw, r, "", state) {
		return
	}

	// This will send a correct 404
	h.h.ServeHTTP(rw, r)
}

func (h *handler) serveHTML(rw http.ResponseWriter, r *http.Request, reqPath string, state htmlState) bool {
	if data, err := h.htmlFiles.renderWithState(reqPath, state); err == nil {
		if reqPath == "" {
			// Pass "index.html" to the ServeContent so the ServeContent sets the right content headers.
			reqPath = "index.html"
		}
		http.ServeContent(rw, r, reqPath, time.Time{}, bytes.NewReader(data))
		return true
	}
	return false
}

type htmlTemplates struct {
	tpls *template.Template
}

// renderWithState will render the file using the given nonce if the file exists
// as a template. If it does not, it will return an error.
func (t *htmlTemplates) renderWithState(filePath string, state htmlState) ([]byte, error) {
	var buf bytes.Buffer
	if filePath == "" {
		filePath = "index.html"
	}
	err := t.tpls.ExecuteTemplate(&buf, filePath, state)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
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

// htmlFiles recursively walks the file system passed finding all *.html files.
// The template returned has all html files parsed.
func htmlFiles(files fs.FS) (*htmlTemplates, error) {
	// root is the collection of html templates. All templates are named by their pathing.
	// So './404.html' is named '404.html'. './subdir/index.html' is 'subdir/index.html'
	root := template.New("")

	rootPath := "."
	err := fs.WalkDir(files, rootPath, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if dirEntry.IsDir() {
			return nil
		}

		if filepath.Ext(dirEntry.Name()) != ".html" {
			return nil
		}

		file, err := files.Open(path)
		if err != nil {
			return err
		}

		data, err := io.ReadAll(file)
		if err != nil {
			return err
		}

		tPath := strings.TrimPrefix(path, rootPath+string(filepath.Separator))
		_, err = root.New(tPath).Parse(string(data))
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &htmlTemplates{
		tpls: root,
	}, nil
}
