package agentpreview

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/cryptorand"
)

// previewWriteTimeout replaces the agent API's 20s write timeout for preview
// responses, so large assets can finish sending.
const previewWriteTimeout = 5 * time.Minute

// windowsUnsafeNameChars are characters that let a requested name reach
// another file on Windows: a backslash is a path separator, a colon names an
// alternate data stream, and a tilde appears in short names such as HIDDEN~1
// for .hidden. Other systems allow them in file names, as in vendors~main.js.
const windowsUnsafeNameChars = `\:~`

var (
	errNotServed     = xerrors.New("previews serve only regular files and do not follow symlinks")
	errNameNotServed = xerrors.New("previews do not serve path elements that are empty, start with a dot, or on Windows contain any of " + windowsUnsafeNameChars)
)

// preview is a published path. Each request opens dir again, so a directory
// that is deleted and rebuilt is served without publishing it again.
type preview struct {
	dir string
	// file is the published file's name in dir for file previews, and empty
	// for directory previews.
	file string
}

// publish publishes an .html file or a directory with index.html, and returns
// its preview ID and the cleaned path it serves. Publishing a path again keeps
// its ID.
func (api *API) publish(name string) (id, cleaned string, err error) {
	if !filepath.IsAbs(name) {
		return "", "", xerrors.New("preview path must be absolute")
	}
	cleaned = filepath.Clean(name)

	info, err := os.Stat(cleaned)
	if err != nil {
		return "", "", err
	}

	p := preview{dir: cleaned}
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Ext(cleaned), ".html") {
			return "", "", xerrors.New("preview requires an .html file or a directory with index.html")
		}
		p = preview{dir: filepath.Dir(cleaned), file: filepath.Base(cleaned)}
	}

	first, err := p.servedName("")
	if err != nil {
		return "", "", err
	}
	f, err := openServed(p.dir, first)
	if err != nil {
		return "", "", xerrors.Errorf("%s: %w", first, err)
	}
	_ = f.Close()

	api.mu.Lock()
	defer api.mu.Unlock()

	id, ok := api.idsByPath[cleaned]
	if !ok {
		// A random ID makes a link from before an agent restart 404 instead of
		// serving another path. Preview hostnames require eight hex digits.
		for {
			id, err = cryptorand.HexString(8)
			if err != nil {
				return "", "", xerrors.Errorf("generate preview ID: %w", err)
			}
			if _, taken := api.previews[id]; !taken {
				break
			}
		}
		api.idsByPath[cleaned] = id
	}
	api.previews[id] = p

	return id, cleaned, nil
}

func (api *API) handleServe(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Cache-Control", "no-store")

	// Take the file path from the decoded URL path, because chi route
	// parameters keep the request's percent-encoding.
	id := chi.URLParam(r, "id")
	_, name, _ := strings.Cut(r.URL.Path, "/"+id+"/")

	f, name, err := api.open(id, name)
	if err != nil {
		// Files a preview refuses to serve get the same 404 as missing files,
		// so pages can't tell them apart. The request log says why.
		if requestLogger := loggermw.RequestLoggerFromContext(r.Context()); requestLogger != nil {
			requestLogger.WithFields(slog.Error(err))
		}
		http.NotFound(rw, r)
		return
	}
	defer f.Close()

	if err := http.NewResponseController(rw).SetWriteDeadline(time.Now().Add(previewWriteTimeout)); err != nil {
		api.logger.Warn(r.Context(), "extend preview write deadline", slog.Error(err))
	}
	http.ServeContent(rw, r, name, time.Time{}, f)
}

// open opens the file that a request for name in preview id serves, and
// returns the name it opened.
func (api *API) open(id, name string) (*os.File, string, error) {
	api.mu.RLock()
	p, ok := api.previews[id]
	api.mu.RUnlock()
	if !ok {
		return nil, "", fs.ErrNotExist
	}

	name, err := p.servedName(name)
	if err != nil {
		return nil, "", err
	}
	f, err := openServed(p.dir, name)
	return f, name, err
}

// servedName returns the slash-separated name in p.dir that a request for
// name serves: the published file, or index.html for a directory path.
func (p preview) servedName(name string) (string, error) {
	if p.file != "" {
		if name != "" {
			return "", fs.ErrNotExist
		}
		return p.file, nil
	}

	if name == "" || strings.HasSuffix(name, "/") {
		name += "index.html"
	}
	if !fs.ValidPath(name) {
		return "", errNameNotServed
	}
	// Names that start with a dot, such as .git and .env, are not served.
	for component := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(component, ".") ||
			runtime.GOOS == "windows" && strings.ContainsAny(component, windowsUnsafeNameChars) {
			return "", errNameNotServed
		}
	}
	return name, nil
}

// openServed opens the slash-separated name in dir. os.Root keeps the open
// inside dir but follows symlinks within it, which could alias a hidden or
// unpublished file, so no path element may be a symlink. The file must be a
// regular file.
func openServed(dir, name string) (*os.File, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	var (
		prefix string
		info   fs.FileInfo
	)
	for component := range strings.SplitSeq(name, "/") {
		prefix = path.Join(prefix, component)
		info, err = root.Lstat(prefix)
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, errNotServed
		}
	}
	if !info.Mode().IsRegular() {
		return nil, errNotServed
	}

	// O_NONBLOCK keeps a FIFO that replaced the file after the Lstat above
	// from blocking the open.
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
