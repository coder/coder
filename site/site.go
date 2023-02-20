package site

import (
	"archive/tar"
	"bytes"
	"crypto/sha1" //#nosec // Not used for cryptography.
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template" // html/template escapes some nonces
	"time"

	"github.com/justinas/nosurf"
	"github.com/klauspost/compress/zstd"
	"github.com/unrolled/secure"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// We always embed the error page HTML because it it doesn't need to be built,
// and it's tiny and doesn't contribute much to the binary size.
var (
	//go:embed static/error.html
	errorHTML string

	errorTemplate *htmltemplate.Template
)

func init() {
	var err error
	errorTemplate, err = htmltemplate.New("error").Parse(errorHTML)
	if err != nil {
		panic(err)
	}
}

// Handler returns an HTTP handler for serving the static site.
func Handler(siteFS fs.FS, binFS http.FileSystem, binHashes map[string]string) http.Handler {
	// html files are handled by a text/template. Non-html files
	// are served by the default file server.
	//
	// REMARK: text/template is needed to inject values on each request like
	//         CSRF.
	files, err := htmlFiles(siteFS)
	if err != nil {
		panic(xerrors.Errorf("Failed to return handler for static files. Html files failed to load: %w", err))
	}

	binHashCache := newBinHashCache(binFS, binHashes)

	mux := http.NewServeMux()
	mux.Handle("/bin/", http.StripPrefix("/bin", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Convert underscores in the filename to hyphens. We eventually want to
		// change our hyphen-based filenames to underscores, but we need to
		// support both for now.
		r.URL.Path = strings.ReplaceAll(r.URL.Path, "_", "-")

		// Set ETag header to the SHA1 hash of the file contents.
		name := filePath(r.URL.Path)
		if name == "" || name == "/" {
			// Serve the directory listing.
			http.FileServer(binFS).ServeHTTP(rw, r)
			return
		}
		if strings.Contains(name, "/") {
			// We only serve files from the root of this directory, so avoid any
			// shenanigans by blocking slashes in the URL path.
			http.NotFound(rw, r)
			return
		}
		hash, err := binHashCache.getHash(name)
		if xerrors.Is(err, os.ErrNotExist) {
			http.NotFound(rw, r)
			return
		}
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		// ETag header needs to be quoted.
		rw.Header().Set("ETag", fmt.Sprintf(`%q`, hash))

		// http.FileServer will see the ETag header and automatically handle
		// If-Match and If-None-Match headers on the request properly.
		http.FileServer(binFS).ServeHTTP(rw, r)
	})))
	mux.Handle("/", http.FileServer(http.FS(siteFS))) // All other non-html static files.

	return secureHeaders(&handler{
		fs:        siteFS,
		htmlFiles: files,
		h:         mux,
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
	// for deny-listed items enumerated here. The reason for this approach is that
	// cache invalidation techniques should be used by default for all build
	// processed assets. The scenarios where we don't use cache invalidation
	// techniques are one-offs or things that should have invalidation in the
	// future.
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

	switch {
	// If requesting binaries, serve straight up.
	case reqFile == "bin" || strings.HasPrefix(reqFile, "bin/"):
		h.h.ServeHTTP(resp, req)
		return
	// If the original file path exists we serve it.
	case h.exists(reqFile):
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
	CSPDirectiveWorkerSrc   = "worker-src"
)

func cspHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Content-Security-Policy disables loading certain content types and can prevent XSS injections.
		// This site helps eval your policy for syntax and other common issues: https://csp-evaluator.withgoogle.com/
		// If we ever want to render something like a PDF, we need to adjust "object-src"
		//
		//	The list of CSP options: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Security-Policy/default-src
		cspSrcs := CSPDirectives{
			// All omitted fetch csp srcs default to this.
			CSPDirectiveDefaultSrc: {"'self'"},
			CSPDirectiveConnectSrc: {"'self'"},
			CSPDirectiveChildSrc:   {"'self'"},
			// https://cdn.jsdelivr.net is used by monaco editor on FE for Syntax Highlight
			// https://github.com/suren-atoyan/monaco-react/issues/168
			CSPDirectiveScriptSrc: {"'self' https://cdn.jsdelivr.net"},
			CSPDirectiveStyleSrc:  {"'self' 'unsafe-inline' https://cdn.jsdelivr.net"},
			// data: is used by monaco editor on FE for Syntax Highlight
			CSPDirectiveFontSrc: {"'self' data:"},
			// object-src is needed to support code-server
			CSPDirectiveObjectSrc: {"'self'"},
			// blob: for loading the pwa manifest for code-server
			CSPDirectiveManifestSrc: {"'self' blob:"},
			CSPDirectiveFrameSrc:    {"'self'"},
			// data: for loading base64 encoded icons for generic applications.
			// https: allows loading images from external sources. This is not ideal
			// 	but is required for the templates page that renders readmes.
			//	We should find a better solution in the future.
			CSPDirectiveImgSrc:     {"'self' https: data:"},
			CSPDirectiveFormAction: {"'self'"},
			CSPDirectiveMediaSrc:   {"'self'"},
			// Report all violations back to the server to log
			CSPDirectiveReportURI: {"/api/v2/csp/reports"},
			CSPFrameAncestors:     {"'none'"},

			// Only scripts can manipulate the dom. This prevents someone from
			// naming themselves something like '<svg onload="alert(/cross-site-scripting/)" />'.
			// "require-trusted-types-for" : []string{"'script'"},
		}

		// This extra connect-src addition is required to support old webkit
		// based browsers (Safari).
		// See issue: https://github.com/w3c/webappsec-csp/issues/7
		// Once webkit browsers support 'self' on connect-src, we can remove this.
		// When we remove this, the csp header can be static, as opposed to being
		// dynamically generated for each request.
		host := r.Host
		// It is important r.Host is not an empty string.
		if host != "" {
			// We can add both ws:// and wss:// as browsers do not let https
			// pages to connect to non-tls websocket connections. So this
			// supports both http & https webpages.
			cspSrcs.Append(CSPDirectiveConnectSrc, fmt.Sprintf("wss://%[1]s ws://%[1]s", host))
		}

		var csp strings.Builder
		for src, vals := range cspSrcs {
			_, _ = fmt.Fprintf(&csp, "%s %s; ", src, strings.Join(vals, " "))
		}

		w.Header().Set("Content-Security-Policy", csp.String())
		next.ServeHTTP(w, r)
	})
}

// secureHeaders is only needed for statically served files. We do not need this for api endpoints.
// It adds various headers to enforce browser security features.
func secureHeaders(next http.Handler) http.Handler {
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
		PermissionsPolicy: permissions,

		// Prevent the browser from sending Referrer header with requests
		ReferrerPolicy: "no-referrer",
	}).Handler(cspHeaders(next))
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

// ExtractOrReadBinFS checks the provided fs for compressed coder binaries and
// extracts them into dest/bin if found. As a fallback, the provided FS is
// checked for a /bin directory, if it is non-empty it is returned. Finally
// dest/bin is returned as a fallback allowing binaries to be manually placed in
// dest (usually ${CODER_CACHE_DIRECTORY}/site/bin).
//
// Returns a http.FileSystem that serves unpacked binaries, and a map of binary
// name to SHA1 hash. The returned hash map may be incomplete or contain hashes
// for missing files.
func ExtractOrReadBinFS(dest string, siteFS fs.FS) (http.FileSystem, map[string]string, error) {
	if dest == "" {
		// No destination on fs, embedded fs is the only option.
		binFS, err := fs.Sub(siteFS, "bin")
		if err != nil {
			return nil, nil, xerrors.Errorf("cache path is empty and embedded fs does not have /bin: %w", err)
		}
		return http.FS(binFS), nil, nil
	}

	dest = filepath.Join(dest, "bin")
	mkdest := func() (http.FileSystem, error) {
		err := os.MkdirAll(dest, 0o700)
		if err != nil {
			return nil, xerrors.Errorf("mkdir failed: %w", err)
		}
		return http.Dir(dest), nil
	}

	archive, err := siteFS.Open("bin/coder.tar.zst")
	if err != nil {
		if xerrors.Is(err, fs.ErrNotExist) {
			files, err := fs.ReadDir(siteFS, "bin")
			if err != nil {
				if xerrors.Is(err, fs.ErrNotExist) {
					// Given fs does not have a bin directory, serve from cache
					// directory without extracting anything.
					binFS, err := mkdest()
					if err != nil {
						return nil, nil, xerrors.Errorf("mkdest failed: %w", err)
					}
					return binFS, map[string]string{}, nil
				}
				return nil, nil, xerrors.Errorf("site fs read dir failed: %w", err)
			}

			if len(filterFiles(files, "GITKEEP")) > 0 {
				// If there are other files than bin/GITKEEP, serve the files.
				binFS, err := fs.Sub(siteFS, "bin")
				if err != nil {
					return nil, nil, xerrors.Errorf("site fs sub dir failed: %w", err)
				}
				return http.FS(binFS), nil, nil
			}

			// Nothing we can do, serve the cache directory, thus allowing
			// binaries to be placed there.
			binFS, err := mkdest()
			if err != nil {
				return nil, nil, xerrors.Errorf("mkdest failed: %w", err)
			}
			return binFS, map[string]string{}, nil
		}
		return nil, nil, xerrors.Errorf("open coder binary archive failed: %w", err)
	}
	defer archive.Close()

	binFS, err := mkdest()
	if err != nil {
		return nil, nil, err
	}

	shaFiles, err := parseSHA1(siteFS)
	if err != nil {
		return nil, nil, xerrors.Errorf("parse sha1 file failed: %w", err)
	}

	ok, err := verifyBinSha1IsCurrent(dest, siteFS, shaFiles)
	if err != nil {
		return nil, nil, xerrors.Errorf("verify coder binaries sha1 failed: %w", err)
	}
	if !ok {
		n, err := extractBin(dest, archive)
		if err != nil {
			return nil, nil, xerrors.Errorf("extract coder binaries failed: %w", err)
		}
		if n == 0 {
			return nil, nil, xerrors.New("no files were extracted from coder binaries archive")
		}
	}

	return binFS, shaFiles, nil
}

func filterFiles(files []fs.DirEntry, names ...string) []fs.DirEntry {
	var filtered []fs.DirEntry
	for _, f := range files {
		if slices.Contains(names, f.Name()) {
			continue
		}
		filtered = append(filtered, f)
	}
	return filtered
}

// errHashMismatch is a sentinel error used in verifyBinSha1IsCurrent.
var errHashMismatch = xerrors.New("hash mismatch")

func parseSHA1(siteFS fs.FS) (map[string]string, error) {
	b, err := fs.ReadFile(siteFS, "bin/coder.sha1")
	if err != nil {
		return nil, xerrors.Errorf("read coder sha1 from embedded fs failed: %w", err)
	}

	shaFiles := make(map[string]string)
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte{'\n'}) {
		parts := bytes.Split(line, []byte{' ', '*'})
		if len(parts) != 2 {
			return nil, xerrors.Errorf("malformed sha1 file: %w", err)
		}
		shaFiles[string(parts[1])] = strings.ToLower(string(parts[0]))
	}
	if len(shaFiles) == 0 {
		return nil, xerrors.Errorf("empty sha1 file: %w", err)
	}

	return shaFiles, nil
}

func verifyBinSha1IsCurrent(dest string, siteFS fs.FS, shaFiles map[string]string) (ok bool, err error) {
	b1, err := fs.ReadFile(siteFS, "bin/coder.sha1")
	if err != nil {
		return false, xerrors.Errorf("read coder sha1 from embedded fs failed: %w", err)
	}
	b2, err := os.ReadFile(filepath.Join(dest, "coder.sha1"))
	if err != nil {
		if xerrors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, xerrors.Errorf("read coder sha1 failed: %w", err)
	}

	// Check shasum files for equality for early-exit.
	if !bytes.Equal(b1, b2) {
		return false, nil
	}

	var eg errgroup.Group
	// Speed up startup by verifying files concurrently. Concurrency
	// is limited to save resources / early-exit. Early-exit speed
	// could be improved by using a context aware io.Reader and
	// passing the context from errgroup.WithContext.
	eg.SetLimit(3)

	// Verify the hash of each on-disk binary.
	for file, hash1 := range shaFiles {
		file := file
		hash1 := hash1
		eg.Go(func() error {
			hash2, err := sha1HashFile(filepath.Join(dest, file))
			if err != nil {
				if xerrors.Is(err, fs.ErrNotExist) {
					return errHashMismatch
				}
				return xerrors.Errorf("hash file failed: %w", err)
			}
			if !strings.EqualFold(hash1, hash2) {
				return errHashMismatch
			}
			return nil
		})
	}
	err = eg.Wait()
	if err != nil {
		if xerrors.Is(err, errHashMismatch) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// sha1HashFile computes a SHA1 hash of the file, returning the hex
// representation.
func sha1HashFile(name string) (string, error) {
	//#nosec // Not used for cryptography.
	hash := sha1.New()
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = io.Copy(hash, f)
	if err != nil {
		return "", err
	}

	b := make([]byte, hash.Size())
	hash.Sum(b[:0])

	return hex.EncodeToString(b), nil
}

func extractBin(dest string, r io.Reader) (numExtracted int, err error) {
	opts := []zstd.DOption{
		// Concurrency doesn't help us when decoding the tar and
		// can actually slow us down.
		zstd.WithDecoderConcurrency(1),
		// Ignoring checksums can give a slight performance
		// boost but it's probably not worth the reduced safety.
		zstd.IgnoreChecksum(false),
		// Allow the decoder to use more memory giving us a 2-3x
		// performance boost.
		zstd.WithDecoderLowmem(false),
	}
	zr, err := zstd.NewReader(r, opts...)
	if err != nil {
		return 0, xerrors.Errorf("open zstd archive failed: %w", err)
	}
	defer zr.Close()

	tr := tar.NewReader(zr)
	n := 0
	for {
		h, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return n, nil
			}
			return n, xerrors.Errorf("read tar archive failed: %w", err)
		}
		if h.Name == "." || strings.Contains(h.Name, "..") {
			continue
		}

		name := filepath.Join(dest, filepath.Base(h.Name))
		f, err := os.Create(name)
		if err != nil {
			return n, xerrors.Errorf("create file failed: %w", err)
		}
		//#nosec // We created this tar, no risk of decompression bomb.
		_, err = io.Copy(f, tr)
		if err != nil {
			_ = f.Close()
			return n, xerrors.Errorf("write file contents failed: %w", err)
		}
		err = f.Close()
		if err != nil {
			return n, xerrors.Errorf("close file failed: %w", err)
		}

		n++
	}
}

// ErrorPageData contains the variables that are found in
// site/static/error.html.
type ErrorPageData struct {
	Status       int
	Title        string
	Description  string
	RetryEnabled bool
	DashboardURL string
}

// RenderStaticErrorPage renders the static error page. This is used by app
// requests to avoid dependence on the dashboard but maintain the ability to
// render a friendly error page on subdomains.
func RenderStaticErrorPage(rw http.ResponseWriter, r *http.Request, data ErrorPageData) {
	type outerData struct {
		Error ErrorPageData
	}

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(data.Status)

	err := errorTemplate.Execute(rw, outerData{Error: data})
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to render error page: " + err.Error(),
			Detail:  fmt.Sprintf("Original error was: %d %s, %s", data.Status, data.Title, data.Description),
		})
		return
	}
}

type binHashCache struct {
	binFS http.FileSystem

	hashes map[string]string
	mut    sync.RWMutex
	sf     singleflight.Group
	sem    chan struct{}
}

func newBinHashCache(binFS http.FileSystem, binHashes map[string]string) *binHashCache {
	b := &binHashCache{
		binFS:  binFS,
		hashes: make(map[string]string, len(binHashes)),
		mut:    sync.RWMutex{},
		sf:     singleflight.Group{},
		sem:    make(chan struct{}, 4),
	}
	// Make a copy since we're gonna be mutating it.
	for k, v := range binHashes {
		b.hashes[k] = v
	}

	return b
}

func (b *binHashCache) getHash(name string) (string, error) {
	b.mut.RLock()
	hash, ok := b.hashes[name]
	b.mut.RUnlock()
	if ok {
		return hash, nil
	}

	// Avoid DOS by using a pool, and only doing work once per file.
	v, err, _ := b.sf.Do(name, func() (interface{}, error) {
		b.sem <- struct{}{}
		defer func() { <-b.sem }()

		f, err := b.binFS.Open(name)
		if err != nil {
			return "", err
		}
		defer f.Close()

		h := sha1.New() //#nosec // Not used for cryptography.
		_, err = io.Copy(h, f)
		if err != nil {
			return "", err
		}

		hash := hex.EncodeToString(h.Sum(nil))
		b.mut.Lock()
		b.hashes[name] = hash
		b.mut.Unlock()
		return hash, nil
	})
	if err != nil {
		return "", err
	}

	//nolint:forcetypeassert
	return strings.ToLower(v.(string)), nil
}
