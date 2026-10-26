package agentpreview

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
	"github.com/coder/coder/v2/cryptorand"
)

// maxPreviews is the most previews the agent publishes until it restarts.
const maxPreviews = 64

// previewWriteTimeout replaces the agent API's 20s write timeout for preview
// responses, so large assets can finish sending.
const previewWriteTimeout = 5 * time.Minute

// unsafeNameChars are characters that let a requested name reach another
// file on Windows: a backslash is a path separator, a colon names an
// alternate data stream, and a tilde appears in short names such as HIDDEN~1
// for .hidden.
const unsafeNameChars = `\:~`

var (
	errPreviewsClosed = xerrors.New("previews are closed")
	// errPathUnreadable replaces OS errors, which can name other paths
	// such as symlink targets, in responses that reach the chat's AI model.
	errPathUnreadable = xerrors.New("preview path does not exist or cannot be read")
	errNotServed      = xerrors.New("previews serve only regular files without other hard links, and do not follow symlinks")
	errNameNotServed  = xerrors.New("previews do not serve path elements that are empty, start with a dot, are not valid UTF-8, or contain any of " + unsafeNameChars)
)

type preview struct {
	// file is the published file's name for file previews, empty for
	// directory previews.
	file string
	root *os.Root
}

// publish publishes a file or directory and returns its preview ID and the
// canonical path it serves. Publishing a path again keeps its ID and serves
// the path's current contents.
func (api *API) publish(name string) (id, canonical string, err error) {
	if !filepath.IsAbs(name) {
		return "", "", xerrors.New("preview path must be absolute")
	}

	// Log the OS errors that responses hide.
	unreadable := func(err error) error {
		api.logger.Debug(context.Background(), "preview path unreadable", slog.F("path", name), slog.Error(err))
		return errPathUnreadable
	}

	// EvalSymlinks can fail on Windows when it looks up the case of each path
	// element, as it does on the CI RAM disk. Falling back is safe because the
	// opens below follow directory symlinks and reject a symlinked file. Two
	// paths to the same file may then get different IDs.
	canonical, err = filepath.EvalSymlinks(name)
	if err != nil {
		canonical = filepath.Clean(name)
	}

	info, err := os.Stat(canonical)
	if err != nil {
		return "", "", unreadable(err)
	}

	rootName, file, initial := canonical, "", "index.html"
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Ext(canonical), ".html") {
			return "", "", xerrors.New("preview requires an .html file or a directory with index.html")
		}

		rootName, file = filepath.Dir(canonical), filepath.Base(canonical)
		initial = file
	}

	root, err := openPreviewRoot(rootName)
	if err != nil {
		return "", "", unreadable(err)
	}

	p := &preview{file: file, root: root}
	f, err := p.openFile(initial)
	if err != nil {
		_ = root.Close()
		if !errors.Is(err, errNotServed) {
			err = unreadable(err)
		}
		return "", "", xerrors.Errorf("%s: %w", initial, err)
	}
	_ = f.Close()

	api.mu.Lock()
	defer api.mu.Unlock()

	if api.closed {
		_ = root.Close()
		return "", "", errPreviewsClosed
	}

	id, ok := api.idsByPath[canonical]
	if !ok {
		if len(api.idsByPath) >= maxPreviews {
			_ = root.Close()
			return "", "", xerrors.Errorf("at most %d previews can be published until the workspace agent restarts", maxPreviews)
		}

		// A random ID makes a link from before an agent restart 404 instead of
		// serving another path. Preview hostnames require eight hex digits.
		for id == "" || api.previews[id] != nil {
			id, err = cryptorand.HexString(8)
			if err != nil {
				_ = root.Close()
				return "", "", xerrors.Errorf("generate preview ID: %w", err)
			}
		}
		api.idsByPath[canonical] = id
	}

	if previous := api.previews[id]; previous != nil {
		_ = previous.root.Close()
	}
	api.previews[id] = p

	return id, canonical, nil
}

func (api *API) handleServe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")

	// Take the file path from the decoded URL path, because chi route
	// parameters keep the request's percent-encoding.
	id := chi.URLParam(r, "id")
	_, name, _ := strings.Cut(r.URL.Path, "/"+id+"/")

	f, name, err := api.openServed(id, name)
	// Files a preview refuses to serve get the same 404 as missing files, so
	// pages can't tell them apart. The log line says why.
	if requestLogger := loggermw.RequestLoggerFromContext(r.Context()); err != nil && requestLogger != nil {
		requestLogger.WithFields(slog.Error(err))
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errNotServed) || errors.Is(err, errNameNotServed) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Could not read the preview file.", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(previewWriteTimeout)); err != nil {
		api.logger.Warn(r.Context(), "extend preview write deadline", slog.Error(err))
	}
	http.ServeContent(w, r, name, time.Time{}, f)
}

// openServed opens the file a request for name in preview id serves: the
// published file, or index.html for a directory path. It returns the name it
// opened.
func (api *API) openServed(id, name string) (*os.File, string, error) {
	// Republishing closes the replaced os.Root, so hold the read lock until
	// openFile has made its calls on it.
	api.mu.RLock()
	defer api.mu.RUnlock()

	p := api.previews[id]
	if p == nil {
		return nil, "", fs.ErrNotExist
	}

	if p.file != "" {
		if name != "" {
			return nil, "", fs.ErrNotExist
		}
		f, err := p.openFile(p.file)
		return f, p.file, err
	}

	if name == "" || strings.HasSuffix(name, "/") {
		name += "index.html"
	}

	if !fs.ValidPath(name) {
		return nil, "", errNameNotServed
	}

	// Names inside a directory preview that start with a dot, such as .git and
	// .env, are not served. HFS+ ignores format characters such as U+200C when
	// comparing names, so they are trimmed before the check.
	for component := range strings.SplitSeq(name, "/") {
		visible := strings.TrimLeftFunc(component, func(r rune) bool { return unicode.Is(unicode.Cf, r) })
		if strings.HasPrefix(visible, ".") || strings.ContainsAny(component, unsafeNameChars) {
			return nil, "", errNameNotServed
		}
	}

	f, err := p.openFile(name)
	return f, name, err
}

// openFile opens the slash-separated path name inside the preview. It rejects
// symlinks, which could alias hidden or unpublished files, and hard-linked
// files, which may have another link outside the preview.
func (p *preview) openFile(name string) (*os.File, error) {
	dir := p.root
	defer func() {
		if dir != p.root {
			_ = dir.Close()
		}
	}()

	components := strings.Split(name, "/")
	for _, component := range components[:len(components)-1] {
		// Comparing the opened directory with the Lstat result rejects a
		// directory that was replaced with a symlink between the two calls.
		info, err := dir.Lstat(component)
		if err != nil || info.Mode().IsRegular() {
			return nil, fs.ErrNotExist
		}
		if !info.IsDir() {
			return nil, errNotServed
		}

		next, err := openPreviewSubroot(dir, component)
		if err != nil {
			return nil, err
		}

		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			_ = next.Close()
			return nil, errNotServed
		}

		if dir != p.root {
			_ = dir.Close()
		}
		dir = next
	}

	leaf := components[len(components)-1]
	info, err := dir.Lstat(leaf)
	if err != nil {
		return nil, fs.ErrNotExist
	}
	if !info.Mode().IsRegular() {
		return nil, errNotServed
	}

	// O_NONBLOCK keeps a FIFO that replaced the file after the Lstat above
	// from blocking. os.Root follows in-root symlinks even with O_NOFOLLOW, so
	// symlinks are rejected with Lstat and SameFile instead.
	f, err := dir.OpenFile(leaf, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) || isHardLinked(f, opened) {
		_ = f.Close()
		return nil, errNotServed
	}

	return f, nil
}

func openPreviewRoot(name string) (*os.Root, error) {
	parentName := filepath.Dir(name)
	if runtime.GOOS != "windows" || parentName == name {
		return os.OpenRoot(name + "/.")
	}

	// On Windows, os.OpenRoot opens name without FILE_SHARE_DELETE, which
	// would stop the directory from being renamed or deleted while it is
	// published. Root.OpenRoot shares delete, so open name from its parent.
	parent, err := os.OpenRoot(parentName)
	if err != nil {
		return nil, err
	}
	defer parent.Close()

	return openPreviewSubroot(parent, filepath.Base(name))
}

func openPreviewSubroot(root *os.Root, name string) (*os.Root, error) {
	// On Unix, os.OpenRoot and Root.OpenRoot open the final path element
	// without O_DIRECTORY, so a FIFO there would block. Appending "/." makes
	// the final element a parent directory, which os opens with O_DIRECTORY.
	// filepath.Join and filepath.Clean would drop the "/.".
	return root.OpenRoot(name + "/.")
}
