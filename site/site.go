package site

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha1" //#nosec // Not used for cryptography.
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template" // html/template escapes some nonces
	"time"

	"github.com/justinas/nosurf"
	"github.com/klauspost/compress/zstd"
	"github.com/unrolled/secure"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

type apiResponseContextKey struct{}

// WithAPIResponse returns a context with the APIResponse value attached.
// This is used to inject API response data to the index.html for additional
// metadata in error pages.
func WithAPIResponse(ctx context.Context, apiResponse APIResponse) context.Context {
	return context.WithValue(ctx, apiResponseContextKey{}, apiResponse)
}

// Handler returns an HTTP handler for serving the static site.
func Handler(siteFS fs.FS, binFS http.FileSystem) http.Handler {
	// html files are handled by a text/template. Non-html files
	// are served by the default file server.
	//
	// REMARK: text/template is needed to inject values on each request like
	//         CSRF.
	files, err := htmlFiles(siteFS)
	if err != nil {
		panic(xerrors.Errorf("Failed to return handler for static files. Html files failed to load: %w", err))
	}

	mux := http.NewServeMux()
	mux.Handle("/bin/", http.StripPrefix("/bin", http.FileServer(binFS)))
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
	APIResponse APIResponse
	CSP         cspState
	CSRF        csrfState
}

type APIResponse struct {
	StatusCode int
	Message    string
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

	apiResponseRaw := req.Context().Value(apiResponseContextKey{})
	if apiResponseRaw != nil {
		apiResponse, ok := apiResponseRaw.(APIResponse)
		if !ok {
			panic("dev error: api response in context isn't the correct type")
		}
		state.APIResponse = apiResponse
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

// ExtractOrReadBinFS checks the provided fs for compressed coder
// binaries and extracts them into dest/bin if found. As a fallback,
// the provided FS is checked for a /bin directory, if it is non-empty
// it is returned. Finally dest/bin is returned as a fallback allowing
// binaries to be manually placed in dest (usually
// ${CODER_CACHE_DIRECTORY}/site/bin).
func ExtractOrReadBinFS(dest string, siteFS fs.FS) (http.FileSystem, error) {
	if dest == "" {
		// No destination on fs, embedded fs is the only option.
		binFS, err := fs.Sub(siteFS, "bin")
		if err != nil {
			return nil, xerrors.Errorf("cache path is empty and embedded fs does not have /bin: %w", err)
		}
		return http.FS(binFS), nil
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
					// Given fs does not have a bin directory,
					// serve from cache directory.
					return mkdest()
				}
				return nil, xerrors.Errorf("site fs read dir failed: %w", err)
			}

			if len(filterFiles(files, "GITKEEP")) > 0 {
				// If there are other files than bin/GITKEEP,
				// serve the files.
				binFS, err := fs.Sub(siteFS, "bin")
				if err != nil {
					return nil, xerrors.Errorf("site fs sub dir failed: %w", err)
				}
				return http.FS(binFS), nil
			}

			// Nothing we can do, serve the cache directory,
			// thus allowing binaries to be places there.
			return mkdest()
		}
		return nil, xerrors.Errorf("open coder binary archive failed: %w", err)
	}
	defer archive.Close()

	dir, err := mkdest()
	if err != nil {
		return nil, err
	}

	ok, err := verifyBinSha1IsCurrent(dest, siteFS)
	if err != nil {
		return nil, xerrors.Errorf("verify coder binaries sha1 failed: %w", err)
	}
	if !ok {
		n, err := extractBin(dest, archive)
		if err != nil {
			return nil, xerrors.Errorf("extract coder binaries failed: %w", err)
		}
		if n == 0 {
			return nil, xerrors.New("no files were extracted from coder binaries archive")
		}
	}

	return dir, nil
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

func verifyBinSha1IsCurrent(dest string, siteFS fs.FS) (ok bool, err error) {
	b1, err := fs.ReadFile(siteFS, "bin/coder.sha1")
	if err != nil {
		return false, xerrors.Errorf("read coder sha1 from embedded fs failed: %w", err)
	}
	// Parse sha1 file.
	shaFiles := make(map[string][]byte)
	for _, line := range bytes.Split(bytes.TrimSpace(b1), []byte{'\n'}) {
		parts := bytes.Split(line, []byte{' ', '*'})
		if len(parts) != 2 {
			return false, xerrors.Errorf("malformed sha1 file: %w", err)
		}
		shaFiles[string(parts[1])] = parts[0]
	}
	if len(shaFiles) == 0 {
		return false, xerrors.Errorf("empty sha1 file: %w", err)
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
			if !bytes.Equal(hash1, hash2) {
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
func sha1HashFile(name string) ([]byte, error) {
	//#nosec // Not used for cryptography.
	hash := sha1.New()
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	_, err = io.Copy(hash, f)
	if err != nil {
		return nil, err
	}

	b := make([]byte, hash.Size())
	hash.Sum(b[:0])

	return []byte(hex.EncodeToString(b)), nil
}

func extractBin(dest string, r io.Reader) (numExtraced int, err error) {
	opts := []zstd.DOption{
		// Concurrency doesn't help us when decoding the tar and
		// can actually slow us down.
		zstd.WithDecoderConcurrency(1),
		// Ignoring checksums can give a slight performance
		// boost but it's probalby not worth the reduced safety.
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
