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
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// windowsUnsafeNameChars are characters that let a requested name reach
// another file on Windows: a backslash is a path separator, a colon names an
// alternate data stream, and a tilde appears in short names such as HIDDEN~1
// for .hidden.
const windowsUnsafeNameChars = `\:~`

var (
	errNotServed     = xerrors.New("previews serve only regular files and do not follow symlinks")
	errNameNotServed = xerrors.New("previews do not serve path elements that are empty, start with a dot, or on Windows contain any of " + windowsUnsafeNameChars)
	errIDTaken       = xerrors.New("another published path has the same preview ID; rename the file or directory and try again")
)

type preview struct {
	dir  string
	file string
}

func (api *API) publish(name string) (workspacesdk.OpenPreviewResponse, error) {
	if !filepath.IsAbs(name) {
		return workspacesdk.OpenPreviewResponse{}, xerrors.New("preview path must be absolute")
	}
	cleaned := filepath.Clean(name)

	info, err := os.Stat(cleaned)
	if err != nil {
		return workspacesdk.OpenPreviewResponse{}, err
	}

	p := preview{dir: cleaned}
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Ext(cleaned), ".html") {
			return workspacesdk.OpenPreviewResponse{}, xerrors.New("preview requires an .html file or a directory with index.html")
		}
		p = preview{dir: filepath.Dir(cleaned), file: filepath.Base(cleaned)}
	}

	first, err := p.servedName("")
	if err != nil {
		return workspacesdk.OpenPreviewResponse{}, err
	}
	f, err := openServed(p.dir, first)
	if err != nil {
		return workspacesdk.OpenPreviewResponse{}, xerrors.Errorf("%s: %w", first, err)
	}
	_ = f.Close()

	id := appurl.NewPreviewID(cleaned)
	api.mu.Lock()
	defer api.mu.Unlock()
	if existing, ok := api.previews[id]; ok && existing != p {
		return workspacesdk.OpenPreviewResponse{}, errIDTaken
	}
	api.previews[id] = p

	return workspacesdk.OpenPreviewResponse{ID: id, Path: cleaned, File: p.file != ""}, nil
}

func (api *API) handleServe(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Cache-Control", "no-store")

	// Take the file path from the decoded URL path, because chi route
	// parameters keep the request's percent-encoding.
	id := chi.URLParam(r, "id")
	_, name, _ := strings.Cut(r.URL.Path, "/"+id+"/")

	f, name, err := api.open(appurl.PreviewID(id), name)
	if err != nil {
		if requestLogger := loggermw.RequestLoggerFromContext(r.Context()); requestLogger != nil {
			requestLogger.WithFields(slog.Error(err))
		}
		http.NotFound(rw, r)
		return
	}
	defer f.Close()

	// Serving a large file can outlast the agent server's 20s WriteTimeout.
	_ = http.NewResponseController(rw).SetWriteDeadline(time.Now().Add(5 * time.Minute))
	http.ServeContent(rw, r, name, time.Time{}, f)
}

func (api *API) open(id appurl.PreviewID, name string) (*os.File, string, error) {
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
	for component := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(component, ".") ||
			runtime.GOOS == "windows" && strings.ContainsAny(component, windowsUnsafeNameChars) {
			return "", errNameNotServed
		}
	}
	return name, nil
}

// os.Root follows symlinks that stay inside dir, which could alias a hidden
// or unpublished file.
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
