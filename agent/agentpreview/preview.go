package agentpreview

import (
	"errors"
	"io"
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

// unpublishedPage answers a preview ID that this agent process has not
// published. Publications live in memory, so a saved preview shows this page
// after the agent restarts.
const unpublishedPage = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Preview unavailable</title></head>
<body><p>This preview is no longer available because the workspace agent restarted. Ask the agent to open it again.</p></body>
</html>
`

var (
	errUnpublished   = xerrors.New("no preview is published with this ID")
	errSymlink       = xerrors.New("previews do not follow symlinks")
	errIsDirectory   = xerrors.New("previews serve files, not directories")
	errNotRegular    = xerrors.New("previews serve only regular files")
	errChanged       = xerrors.New("the file changed while it was being opened; try again")
	errNameNotServed = xerrors.New("previews do not serve path elements that are empty, start with a dot, or on Windows contain any of " + windowsUnsafeNameChars)
)

// preview is a published path. A directory preview has file == "" and dir
// set to the directory; a file preview has dir set to the file's parent.
type preview struct {
	path string
	dir  string
	file string
}

func (api *API) publish(name string) (workspacesdk.OpenPreviewResponse, int, error) {
	if !filepath.IsAbs(name) {
		return workspacesdk.OpenPreviewResponse{}, http.StatusBadRequest, xerrors.Errorf("preview path %q must be absolute", name)
	}
	cleaned := filepath.Clean(name)
	// openRoot refuses a symlinked directory, which includes a file's parent such
	// as macOS /tmp. Resolving the parent once serves a path under a symlinked
	// directory from its real location, and a later symlink swap of the path or of
	// a file preview's parent is refused. A parent that cannot be resolved, as on
	// some Windows volumes, is used as given, so a missing path fails at os.Stat.
	parent, err := filepath.EvalSymlinks(filepath.Dir(cleaned))
	if err != nil {
		parent = filepath.Dir(cleaned)
	}
	resolved := filepath.Join(parent, filepath.Base(cleaned))

	info, err := os.Stat(resolved)
	if err != nil {
		return workspacesdk.OpenPreviewResponse{}, http.StatusBadRequest, err
	}

	p := preview{path: resolved, dir: resolved}
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Ext(resolved), ".html") {
			return workspacesdk.OpenPreviewResponse{}, http.StatusBadRequest, xerrors.New("preview requires an .html file or a directory with index.html")
		}
		p.dir, p.file = parent, filepath.Base(resolved)
	}

	first := p.file
	if first == "" {
		first = "index.html"
	}
	f, err := api.openServed(p.dir, first)
	switch {
	case p.file == "" && errors.Is(err, fs.ErrNotExist):
		err = xerrors.Errorf("directory %s has no index.html; a directory preview needs index.html at its root", p.dir)
	case errors.Is(err, errIsDirectory):
		err = xerrors.Errorf("%s in %s is a directory, not a file", first, p.dir)
	case errors.Is(err, errSymlink):
		err = xerrors.Errorf("%s in %s is a symlink; previews do not follow symlinks", first, p.dir)
	case errors.Is(err, errNotRegular):
		err = xerrors.Errorf("%s in %s is not a regular file", first, p.dir)
	}
	if err != nil {
		return workspacesdk.OpenPreviewResponse{}, http.StatusBadRequest, err
	}
	_ = f.Close()

	id := appurl.NewPreviewID(resolved)
	api.mu.Lock()
	defer api.mu.Unlock()
	if existing, ok := api.previews[id]; ok && existing.path != resolved {
		return workspacesdk.OpenPreviewResponse{}, http.StatusConflict, xerrors.Errorf("preview ID %s is already used by %s; rename or move one of the two paths and publish again", id, existing.path)
	}
	api.previews[id] = p

	return workspacesdk.OpenPreviewResponse{ID: id, Path: resolved, File: p.file != ""}, http.StatusOK, nil
}

func (api *API) handleServe(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Cache-Control", "no-store")

	// Take the file path from the decoded URL path, because chi route
	// parameters keep the request's percent-encoding.
	id := chi.URLParam(r, "id")
	_, name, _ := strings.Cut(r.URL.Path, "/"+id+"/")

	f, served, err := api.open(appurl.PreviewID(id), name)
	if err != nil {
		if errors.Is(err, errIsDirectory) && name != "" && !strings.HasSuffix(name, "/") {
			redirectToDirectory(rw, r)
			return
		}
		requestLogger := loggermw.RequestLoggerFromContext(r.Context())
		if requestLogger != nil {
			requestLogger.WithFields(slog.Error(err))
		}
		if errors.Is(err, errUnpublished) {
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			rw.WriteHeader(http.StatusNotFound)
			if _, err := io.WriteString(rw, unpublishedPage); err != nil && requestLogger != nil {
				requestLogger.WithFields(slog.Error(err))
			}
			return
		}
		http.NotFound(rw, r)
		return
	}
	defer f.Close()

	http.ServeContent(rw, r, served, time.Time{}, f)
}

// redirectToDirectory sends a directory requested without its trailing slash
// to the slash, so relative links in its index.html resolve inside it. The
// Location is relative, because the agent sees its own API path rather than
// the preview host's, and keeps the request's escaping of the last segment.
func redirectToDirectory(rw http.ResponseWriter, r *http.Request) {
	location := path.Base(r.URL.EscapedPath()) + "/"
	if strings.Contains(location, ":") {
		// A colon before the first slash would read as a URL scheme.
		location = "./" + location
	}
	if r.URL.RawQuery != "" {
		location += "?" + r.URL.RawQuery
	}
	rw.Header().Set("Location", location)
	rw.WriteHeader(http.StatusMovedPermanently)
}

func (api *API) open(id appurl.PreviewID, name string) (*os.File, string, error) {
	api.mu.RLock()
	p, ok := api.previews[id]
	api.mu.RUnlock()
	if !ok {
		return nil, "", errUnpublished
	}

	name, err := p.servedName(name)
	if err != nil {
		return nil, "", err
	}
	f, err := api.openServed(p.dir, name)
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

// openServed opens the regular file name inside dir. os.Root follows symlinks
// that stay inside dir, which could alias a hidden or unpublished file. So
// openServed refuses a dir that is a symlink, then resolves name one element
// at a time from the directory opened before it, refusing an element that is
// a symlink or junction or whose opened file is not the one its Lstat checked.
func (api *API) openServed(dir, name string) (*os.File, error) {
	root, err := openRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	elements := strings.Split(name, "/")
	parents, last := elements[:len(elements)-1], elements[len(elements)-1]
	var prefix string
	for _, element := range parents {
		prefix = path.Join(prefix, element)
		info, err := api.lstatElement(root, element, prefix)
		if err != nil {
			return nil, err
		}
		sub, err := root.OpenRoot(element)
		if err != nil {
			return nil, err
		}
		_ = root.Close()
		root = sub
		opened, err := root.Stat(".")
		if err == nil && !os.SameFile(info, opened) {
			err = errChanged
		}
		if err != nil {
			return nil, err
		}
	}

	info, err := api.lstatElement(root, last, path.Join(prefix, last))
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errIsDirectory
	}
	if !info.Mode().IsRegular() {
		return nil, errNotRegular
	}
	// O_NONBLOCK keeps a FIFO that replaced the file after the Lstat above
	// from blocking the open.
	f, err := root.OpenFile(last, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err == nil && !os.SameFile(info, opened) {
		err = errChanged
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// lstatElement returns the Lstat of element in root, refusing a symlink or
// junction, and then runs the beforeOpen hook with prefix, the served path up
// to and including element.
func (api *API) lstatElement(root *os.Root, element, prefix string) (fs.FileInfo, error) {
	info, err := root.Lstat(element)
	if err != nil {
		return nil, err
	}
	if err := checkElement(info.Mode()); err != nil {
		return nil, err
	}
	if api.beforeOpen != nil {
		api.beforeOpen(prefix)
	}
	return info, nil
}

// checkElement refuses a path element that os.Root would follow to another
// file. Go reports a Windows junction as ModeIrregular, not ModeSymlink.
func checkElement(mode fs.FileMode) error {
	switch {
	case mode&fs.ModeSymlink != 0:
		return errSymlink
	case mode&fs.ModeIrregular != 0:
		return errNotRegular
	}
	return nil
}

// openRoot opens dir and refuses it if it is a symlink, which os.OpenRoot
// would follow. Comparing the opened directory with an Lstat of dir, rather
// than checking the Lstat mode alone, also refuses a swap between the two
// calls.
func openRoot(dir string) (*os.Root, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	named, err := os.Lstat(dir)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if !os.SameFile(opened, named) {
		_ = root.Close()
		return nil, xerrors.Errorf("directory %s is a symlink; previews do not follow symlinks", dir)
	}
	return root, nil
}
