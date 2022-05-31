//go:build embed
// +build embed

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
//go:embed out
//go:embed out/bin/*
var site embed.FS

func DefaultHandler() http.Handler {
	// the out directory is where webpack builds are created. It is in the same
	// directory as this file (package site).
	siteFS, err := fs.Sub(site, "out")

	if err != nil {
		// This can't happen... Go would throw a compilation error.
		panic(err)
	}

	return Handler(siteFS)
}

// Handler returns an HTTP handler for serving the static site.
func Handler(fileSystem fs.FS) http.Handler {
	// html files are handled by a text/template. Non-html files
	// are served by the default file server.
	//
	// REMARK: text/template is needed to inject values on each request like
	//         CSRF.
	files, err := htmlFiles(fileSystem)

	if err != nil {
		panic(xerrors.Errorf("Failed to return handler for static files. Html files failed to load: %w", err))
	}

	return secureHeaders(&handler{
		fs:        fileSystem,
		htmlFiles: files,
		h:         http.FileServer(http.FS(fileSystem)), // All other non-html static files
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
func filePath(p string) string {
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

func ShouldCacheFile(reqFile string) bool {
	// Images, favicons and uniquely content hashed bundle assets should be
	// cached. By default, we cache everything in the site/out directory except
	// for deny-listed items enumerated here. The reason for this approach is
	// that cache invalidation techniques should be used by default for all
	// webpack-processed assets. The scenarios where we don't use cache
	// invalidation techniques are one-offs or things that should have
	// invalidation in the future.
	denyListedSuffixes := []string{
		// ALL *.html files
		".html",

		// ALL *worker.js files (including service-worker.js)
		//
		// REMARK(Grey): I'm unsure if there's a desired setting in Workbox for
		//               content hashing these, or if doing so is a risk for
		//               users that have a PWA installed.
		"worker.js",
	}

	for _, suffix := range denyListedSuffixes {
		if strings.HasSuffix(reqFile, suffix) {
			return false
		}
	}

	return true
}

func (h *handler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	// reqFile is the static file requested
	reqFile := filePath(req.URL.Path)
	state := htmlState{
		// Token is the CSRF token for the given request
		CSRF: csrfState{Token: nosurf.Token(req)},
	}

	// First check if it's a file we have in our templates
	if h.serveHTML(resp, req, reqFile, state) {
		return
	}

	// If the original file path exists we serve it.
	if h.exists(reqFile) {
		if ShouldCacheFile(reqFile) {
			resp.Header().Add("Cache-Control", "public, max-age=31536000, immutable")
		}
		h.h.ServeHTTP(resp, req)
		return
	}

	// Serve the file assuming it's an html file
	// This matches paths like `/app/terminal.html`
	req.URL.Path = strings.TrimSuffix(req.URL.Path, "/")
	req.URL.Path += ".html"

	reqFile = filePath(req.URL.Path)
	// All html files should be served by the htmlFile templates
	if h.serveHTML(resp, req, reqFile, state) {
		return
	}

	// If we don't have the file... we should redirect to `/`
	// for our single-page-app.
	req.URL.Path = "/"
	if h.serveHTML(resp, req, "", state) {
		return
	}

	// This will send a correct 404
	h.h.ServeHTTP(resp, req)
}

func (h *handler) serveHTML(resp http.ResponseWriter, request *http.Request, reqPath string, state htmlState) bool {
	if data, err := h.htmlFiles.renderWithState(reqPath, state); err == nil {
		if reqPath == "" {
			// Pass "index.html" to the ServeContent so the ServeContent sets the right content headers.
			reqPath = "index.html"
		}
		http.ServeContent(resp, request, reqPath, time.Time{}, bytes.NewReader(data))
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

// CSPDirectives is a map of all csp fetch directives to their values.
// Each directive is a set of values that is joined by a space (' ').
// All directives are semi-colon separated as a single string for the csp header.
type CSPDirectives map[CSPFetchDirective][]string

func (s CSPDirectives) Append(d CSPFetchDirective, values ...string) {
	if _, ok := s[d]; !ok {
		s[d] = make([]string, 0)
	}
	s[d] = append(s[d], values...)
}

// CSPFetchDirective is the list of all constant fetch directives that
// can be used/appended to.
type CSPFetchDirective string

const (
	CSPDirectiveDefaultSrc  = "default-src"
	CSPDirectiveConnectSrc  = "connect-src"
	CSPDirectiveChildSrc    = "child-src"
	CSPDirectiveScriptSrc   = "script-src"
	CSPDirectiveFontSrc     = "font-src"
	CSPDirectiveStyleSrc    = "style-src"
	CSPDirectiveObjectSrc   = "object-src"
	CSPDirectiveManifestSrc = "manifest-src"
	CSPDirectiveFrameSrc    = "frame-src"
	CSPDirectiveImgSrc      = "img-src"
	CSPDirectiveReportURI   = "report-uri"
	CSPDirectiveFormAction  = "form-action"
	CSPDirectiveMediaSrc    = "media-src"
	CSPFrameAncestors       = "frame-ancestors"
)

// secureHeaders is only needed for statically served files. We do not need this for api endpoints.
// It adds various headers to enforce browser security features.
func secureHeaders(next http.Handler) http.Handler {
	// Content-Security-Policy disables loading certain content types and can prevent XSS injections.
	// This site helps eval your policy for syntax and other common issues: https://csp-evaluator.withgoogle.com/
	// If we ever want to render something like a PDF, we need to adjust "object-src"
	//
	//	The list of CSP options: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Security-Policy/default-src
	cspSrcs := CSPDirectives{
		// All omitted fetch csp srcs default to this.
		CSPDirectiveDefaultSrc: {"'self'"},
		CSPDirectiveConnectSrc: {"'self' ws: wss:"},
		CSPDirectiveChildSrc:   {"'self'"},
		CSPDirectiveScriptSrc:  {"'self'"},
		CSPDirectiveFontSrc:    {"'self'"},
		CSPDirectiveStyleSrc:   {"'self' 'unsafe-inline'"},
		// object-src is needed to support code-server
		CSPDirectiveObjectSrc: {"'self'"},
		// blob: for loading the pwa manifest for code-server
		CSPDirectiveManifestSrc: {"'self' blob:"},
		CSPDirectiveFrameSrc:    {"'self'"},
		// data: for loading base64 encoded icons for generic applications.
		// https: allows loading images from external sources. This is not ideal
		// 	but is required for the templates page that renders readmes.
		//	We should find a better solution in the future.
		CSPDirectiveImgSrc:     {"'self' https: https://cdn.coder.com data:"},
		CSPDirectiveFormAction: {"'self'"},
		CSPDirectiveMediaSrc:   {"'self'"},
		// Report all violations back to the server to log
		CSPDirectiveReportURI: {"/api/v2/csp/reports"},
		CSPFrameAncestors:     {"'none'"},

		// Only scripts can manipulate the dom. This prevents someone from
		// naming themselves something like '<svg onload="alert(/cross-site-scripting/)" />'.
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
	err := fs.WalkDir(files, rootPath, func(filePath string, directory fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if directory.IsDir() {
			return nil
		}

		if filepath.Ext(directory.Name()) != ".html" {
			return nil
		}

		file, err := files.Open(filePath)
		if err != nil {
			return err
		}

		data, err := io.ReadAll(file)
		if err != nil {
			return err
		}

		tPath := strings.TrimPrefix(filePath, rootPath+string(filepath.Separator))
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
