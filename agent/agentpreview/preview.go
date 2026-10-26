package agentpreview

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/xerrors"
)

// maxPreviews bounds the directory handles the agent keeps open. Previews
// cannot be unpublished, so the limit lasts until the agent restarts.
const maxPreviews = 64

var (
	errPreviewsClosed = xerrors.New("previews are closed")
	// errPathUnreadable replaces OS errors, which can name other paths
	// such as symlink targets, in responses to the model.
	errPathUnreadable      = xerrors.New("preview path does not exist or cannot be read")
	errUnsupportedPlatform = xerrors.New("file previews are only supported on Linux and macOS")
)

// preview is one published file or directory. key and file never change
// after open; rootMu guards root and closed. open swaps root on republish.
type preview struct {
	// key is the unhashed identity, used to detect ID collisions.
	key string
	// file is the served base name for file previews, empty for directories.
	file string

	rootMu sync.RWMutex
	root   *os.Root
	closed bool
}

// open publishes a file or directory and returns its preview ID and the
// canonical path it serves. The ID is derived from the canonical path, so
// republishing after a daemon restart keeps the same preview URL.
func (api *API) open(name string) (id string, canonical string, err error) {
	if !filepath.IsAbs(name) {
		return "", "", xerrors.New("preview path must be absolute")
	}

	canonical, err = filepath.EvalSymlinks(name)
	if err != nil {
		return "", "", errPathUnreadable
	}

	info, err := os.Stat(canonical)
	if err != nil {
		return "", "", errPathUnreadable
	}

	directory := info.IsDir()
	rootName, file, initial := canonical, "", "index.html"
	if !directory {
		extension := strings.ToLower(filepath.Ext(canonical))
		if !info.Mode().IsRegular() || (extension != ".html" && extension != ".htm") {
			return "", "", xerrors.New("preview requires a regular HTML file or a directory with index.html")
		}

		rootName, file = filepath.Dir(canonical), filepath.Base(canonical)
		if strings.HasPrefix(file, ".") {
			return "", "", xerrors.New("hidden preview files are not allowed")
		}
		initial = file
	}

	key := fmt.Sprintf("%t:%s", directory, canonical)
	sum := sha256.Sum256([]byte(key))
	id = hex.EncodeToString(sum[:8])

	root, err := openPreviewRoot(rootName)
	if err != nil {
		if errors.Is(err, errors.ErrUnsupported) {
			return "", "", errUnsupportedPlatform
		}
		return "", "", errPathUnreadable
	}

	p := &preview{key: key, file: file, root: root}
	f, err := p.openFile(initial)
	if err != nil {
		_ = root.Close()
		return "", "", xerrors.Errorf("%s does not exist or cannot be read", initial)
	}

	// Confirm we pinned the inode Stat saw, so a swap between EvalSymlinks
	// and opening the root cannot publish a different file or tree.
	var opened os.FileInfo
	if directory {
		opened, err = root.Stat(".")
	} else {
		opened, err = f.Stat()
	}
	_ = f.Close()
	if err != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		return "", "", xerrors.New("preview path changed while opening")
	}

	existing, err := api.add(id, p)
	if err != nil {
		_ = root.Close()
		return "", "", err
	}
	if existing == p {
		return id, canonical, nil
	}

	// Always swap so a rebuilt output directory is served. This happens
	// outside api.mu so waiting for in-flight traversals of this preview
	// does not block requests for other previews. Files those traversals
	// returned stay readable.
	existing.rootMu.Lock()
	if existing.closed {
		existing.rootMu.Unlock()
		_ = root.Close()
		return "", "", errPreviewsClosed
	}
	previous := existing.root
	existing.root = root
	existing.rootMu.Unlock()
	_ = previous.Close()

	return id, canonical, nil
}

// add stores p under id and returns it. If id is already published, add
// returns that preview instead so the caller can swap its root.
func (api *API) add(id string, p *preview) (*preview, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	if api.closed {
		return nil, errPreviewsClosed
	}

	existing := api.previews[id]
	switch {
	case existing == nil && len(api.previews) >= maxPreviews:
		return nil, xerrors.Errorf("at most %d previews can be published until the workspace daemon restarts", maxPreviews)
	case existing == nil:
		api.previews[id] = p
		return p, nil
	case existing.key != p.key:
		return nil, xerrors.New("preview ID collision")
	}

	return existing, nil
}

// ServeHTTP serves published previews at /{id}/. It is only exposed on the
// agent's tailnet preview listener, which coderd reaches through preview app
// hostnames.
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id, name, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")

	api.mu.Lock()
	p := api.previews[id]
	api.mu.Unlock()
	if p == nil {
		http.NotFound(w, r)
		return
	}

	if p.file != "" {
		if name != "" {
			http.NotFound(w, r)
			return
		}
		name = p.file
	} else if name == "" || strings.HasSuffix(name, "/") {
		name += "index.html"
	}

	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00") {
		http.NotFound(w, r)
		return
	}

	// Reject dot components so .git, .env, and similar never leave the
	// workspace.
	for component := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(component, ".") {
			http.NotFound(w, r)
			return
		}
	}

	f, err := p.openFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	// ServeContent answers If-None-Match: * with 304 even without an ETag;
	// drop it so edits always show.
	r.Header.Del("If-None-Match")
	http.ServeContent(noStoreResponseWriter{w}, r, name, time.Time{}, f)
}

// noStoreResponseWriter restores Cache-Control: no-store on every status.
type noStoreResponseWriter struct {
	http.ResponseWriter
}

func (w noStoreResponseWriter) WriteHeader(status int) {
	// ServeContent removes Cache-Control on errors, including invalid ranges.
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}

// Flush and Hijack satisfy the FullResponseWriter lint rule.
func (w noStoreResponseWriter) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w noStoreResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// openFile opens name inside the preview root without following any
// symlink, so links cannot alias hidden or unpublished files. Each
// directory is Lstat'd, opened as its own root, and compared with
// SameFile, which pins it against concurrent renames and rejects a
// directory swapped for a symlink between the Lstat and the open. The
// leaf gets the same Lstat, open, and SameFile treatment, and is rejected
// when it is hard linked, since another link may sit outside the preview.
func (p *preview) openFile(name string) (*os.File, error) {
	p.rootMu.RLock()
	defer p.rootMu.RUnlock()

	// Only the current directory stays open, so deep paths use a constant
	// number of handles.
	dir := p.root
	defer func() {
		if dir != p.root {
			_ = dir.Close()
		}
	}()

	components := strings.Split(name, "/")
	for _, component := range components[:len(components)-1] {
		info, err := dir.Lstat(component)
		if err != nil || !info.IsDir() {
			return nil, fs.ErrNotExist
		}

		next, err := openPreviewSubroot(dir, component)
		if err != nil {
			return nil, err
		}

		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			_ = next.Close()
			return nil, fs.ErrNotExist
		}

		if dir != p.root {
			_ = dir.Close()
		}
		dir = next
	}

	leaf := components[len(components)-1]
	info, err := dir.Lstat(leaf)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fs.ErrNotExist
	}

	f, err := openPreviewFile(dir, leaf)
	if err != nil {
		return nil, err
	}

	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) || isHardLinked(opened) {
		_ = f.Close()
		return nil, fs.ErrNotExist
	}

	return f, nil
}
