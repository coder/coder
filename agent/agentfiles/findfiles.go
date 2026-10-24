package agentfiles

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/filefinder"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	defaultFindFilesLimit = 50
	maxFindFilesLimit     = 200
	// maxFinderRoots bounds the memory used by the file finder index.
	// The least recently used root is evicted when a new one is added.
	maxFinderRoots = 8
)

var errFinderBusy = xerrors.New("the file finder is indexing its maximum number of roots for other requests, retry shortly")

// pendingRoot tracks a root being indexed so concurrent requests for
// the same root wait for one walk instead of starting their own.
type pendingRoot struct {
	done chan struct{}
	err  error
}

// findRoot pairs a requested root with the symlink-resolved directory
// the engine indexes.
type findRoot struct {
	requested string
	real      string
}

// HandleFindFiles fuzzy searches file and directory paths under one or
// more roots. Each root accepts the forms documented on resolvePathForm.
func (api *API) HandleFindFiles(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	parser := httpapi.NewQueryParamParser().RequiredNotEmpty("q")
	q := parser.String(query, "", "q")
	limit := parser.PositiveInt32(query, defaultFindFilesLimit, "limit")
	// Roots are read directly because the list parser splits values on
	// commas, which are valid in paths.
	rawRoots := query["root"]
	parser.Parsed["root"] = true
	parser.ErrorExcessParams(query)
	if len(rawRoots) == 0 {
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  "root",
			Detail: `Query param "root" is required`,
		})
	}
	if len(rawRoots) > maxFinderRoots {
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  "root",
			Detail: fmt.Sprintf(`Query param "root" accepts at most %d values`, maxFinderRoots),
		})
	}
	if limit == 0 || limit > maxFindFilesLimit {
		parser.Errors = append(parser.Errors, codersdk.ValidationError{
			Field:  "limit",
			Detail: fmt.Sprintf(`Query param "limit" must be between 1 and %d`, maxFindFilesLimit),
		})
	}
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	roots := make([]findRoot, 0, len(rawRoots))
	for _, raw := range rawRoots {
		root, status, err := api.resolveFindRoot(raw)
		if err != nil {
			httpapi.Write(ctx, rw, status, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		if !slices.ContainsFunc(roots, func(r findRoot) bool { return r.real == root.real }) {
			roots = append(roots, root)
		}
	}

	results, err := api.findFiles(ctx, roots, q, int(limit))
	if errors.Is(err, errFinderBusy) {
		httpapi.Write(ctx, rw, http.StatusServiceUnavailable, codersdk.Response{
			Message: "File search is busy.",
			Detail:  err.Error(),
		})
		return
	}
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to search files.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.FindFilesResponse{
		Results: results,
	})
}

// resolveFindRoot resolves a root path form, requires an existing
// directory other than the filesystem root, and resolves symlinks for
// indexing.
func (api *API) resolveFindRoot(raw string) (findRoot, int, error) {
	requested, err := api.resolvePathForm(raw)
	if err != nil {
		return findRoot{}, http.StatusInternalServerError, err
	}
	stat, err := api.filesystem.Stat(requested)
	if err != nil {
		return findRoot{}, http.StatusBadRequest, xerrors.Errorf("root %q is not an existing directory: %w", requested, err)
	}
	if !stat.IsDir() {
		return findRoot{}, http.StatusBadRequest, xerrors.Errorf("root %q is not a directory", requested)
	}
	resolved, err := api.resolvePath(requested)
	if err != nil {
		return findRoot{}, http.StatusInternalServerError, xerrors.Errorf("resolve root %q: %w", requested, err)
	}
	if resolved == filepath.Dir(resolved) {
		return findRoot{}, http.StatusBadRequest, xerrors.Errorf("root %q is the filesystem root, choose a narrower root", requested)
	}
	return findRoot{requested: requested, real: resolved}, 0, nil
}

func (api *API) findFiles(ctx context.Context, roots []findRoot, q string, limit int) ([]workspacesdk.FindFilesResult, error) {
	reals := make([]string, 0, len(roots))
	byReal := make(map[string]string, len(roots))
	for _, root := range roots {
		reals = append(reals, root.real)
		byReal[root.real] = root.requested
	}

	engine, release, err := api.ensureFinderRoots(ctx, reals)
	if err != nil {
		return nil, err
	}
	defer release()

	opts := filefinder.DefaultSearchOptions()
	opts.Roots = reals
	// Overlapping roots return the same file once per root, so fetch
	// enough to fill limit after deduplication.
	opts.Limit = limit * len(reals)
	found, err := engine.Search(ctx, q, opts)
	if err != nil {
		return nil, xerrors.Errorf("search: %w", err)
	}

	results := make([]workspacesdk.FindFilesResult, 0, min(len(found), limit))
	seen := make(map[string]struct{}, len(found))
	for _, f := range found {
		if len(results) >= limit {
			break
		}
		requested, ok := byReal[f.Root]
		if !ok {
			continue
		}
		rel := filepath.FromSlash(f.Path)
		realPath := filepath.Join(f.Root, rel)
		if _, ok := seen[realPath]; ok {
			continue
		}
		seen[realPath] = struct{}{}
		results = append(results, workspacesdk.FindFilesResult{
			AbsolutePath: filepath.Join(requested, rel),
			Path:         f.Path,
			Root:         requested,
			IsDir:        f.IsDir,
			Score:        f.Score,
		})
	}
	return results, nil
}

// ensureFinderRoots pins roots, indexes any not yet in the engine, and
// returns the engine with a release func that unpins them. Pinned roots
// are not evicted, so callers must call release after searching. Walks
// run outside finderMu, admitted by the engine's walk limit; concurrent
// requests for the same root wait for the in-flight walk. Indexed and
// pending roots together never exceed maxFinderRoots; when no unpinned
// root can be evicted to make room, errFinderBusy is returned.
func (api *API) ensureFinderRoots(ctx context.Context, roots []string) (*filefinder.Engine, func(), error) {
	api.finderMu.Lock()
	if api.closed {
		api.finderMu.Unlock()
		return nil, nil, filefinder.ErrClosed
	}
	if api.finder == nil {
		api.finder = filefinder.NewEngine(api.logger.Named("filefinder"), filefinder.WithIgnorer(api.getIgnorer()))
		api.finderCtx, api.finderCancel = context.WithCancel(context.Background())
		api.finderPending = make(map[string]*pendingRoot)
		api.finderPins = make(map[string]int)
	}
	engine, finderCtx := api.finder, api.finderCtx
	for _, root := range roots {
		api.finderPins[root]++
	}
	release := func() {
		api.finderMu.Lock()
		defer api.finderMu.Unlock()
		for _, root := range roots {
			api.finderPins[root]--
			if api.finderPins[root] <= 0 {
				delete(api.finderPins, root)
			}
		}
	}
	var adds []string
	var waits []*pendingRoot
	busy := false
	for _, root := range roots {
		if i := slices.Index(api.finderRoots, root); i >= 0 {
			api.finderRoots = append(slices.Delete(api.finderRoots, i, i+1), root)
			continue
		}
		p, ok := api.finderPending[root]
		if !ok {
			if !api.makeFinderRoomLocked(engine) {
				busy = true
				break
			}
			p = &pendingRoot{done: make(chan struct{})}
			api.finderPending[root] = p
			adds = append(adds, root)
		}
		waits = append(waits, p)
	}
	if busy {
		for _, root := range adds {
			p := api.finderPending[root]
			delete(api.finderPending, root)
			p.err = errFinderBusy
			close(p.done)
		}
		api.finderMu.Unlock()
		release()
		return nil, nil, errFinderBusy
	}
	api.finderMu.Unlock()

	for _, root := range adds {
		// The walk uses finderCtx rather than ctx so other requests
		// waiting on the same root are not failed by this request's
		// cancellation; Close cancels it.
		err := engine.AddRoot(finderCtx, root)

		api.finderMu.Lock()
		p := api.finderPending[root]
		delete(api.finderPending, root)
		if err == nil && api.closed {
			err = filefinder.ErrClosed
		}
		if err == nil {
			api.finderRoots = append(api.finderRoots, root)
		} else {
			err = xerrors.Errorf("index root %q: %w", root, err)
		}
		p.err = err
		close(p.done)
		api.finderMu.Unlock()
	}

	for _, p := range waits {
		select {
		case <-p.done:
			if p.err != nil {
				release()
				return nil, nil, p.err
			}
		case <-ctx.Done():
			release()
			return nil, nil, ctx.Err()
		}
	}
	return engine, release, nil
}

// makeFinderRoomLocked evicts least recently used unpinned roots until
// one more root fits within maxFinderRoots, counting pending roots. It
// reports whether there is room. Must be called with finderMu held.
func (api *API) makeFinderRoomLocked(engine *filefinder.Engine) bool {
	for len(api.finderRoots)+len(api.finderPending) >= maxFinderRoots {
		i := slices.IndexFunc(api.finderRoots, func(root string) bool {
			return api.finderPins[root] == 0
		})
		if i < 0 {
			return false
		}
		evict := api.finderRoots[i]
		api.finderRoots = slices.Delete(api.finderRoots, i, i+1)
		if err := engine.RemoveRoot(evict); err != nil {
			api.logger.Warn(context.Background(), "evict file finder root", slog.F("root", evict), slog.Error(err))
		}
	}
	return true
}
