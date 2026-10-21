package agentfiles

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"

	"golang.org/x/xerrors"

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

	roots := make([]string, 0, len(rawRoots))
	for _, raw := range rawRoots {
		root, err := api.resolvePathForm(raw)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		stat, err := api.filesystem.Stat(root)
		if err != nil || !stat.IsDir() {
			detail := "not a directory"
			if err != nil {
				detail = err.Error()
			}
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Root %q is not an existing directory.", root),
				Detail:  detail,
			})
			return
		}
		if !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}

	results, err := api.findFiles(ctx, roots, q, int(limit))
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

func (api *API) findFiles(ctx context.Context, roots []string, q string, limit int) ([]workspacesdk.FindFilesResult, error) {
	// The lock is held through the search so a concurrent request
	// cannot evict a root between indexing and searching it.
	api.finderMu.Lock()
	defer api.finderMu.Unlock()
	if api.closed {
		return nil, filefinder.ErrClosed
	}
	if api.finder == nil {
		api.finder = filefinder.NewEngine(api.logger.Named("filefinder"))
	}

	for _, root := range roots {
		if i := slices.Index(api.finderRoots, root); i >= 0 {
			api.finderRoots = slices.Delete(api.finderRoots, i, i+1)
			api.finderRoots = append(api.finderRoots, root)
		}
	}
	for _, root := range roots {
		if slices.Contains(api.finderRoots, root) {
			continue
		}
		for len(api.finderRoots) >= maxFinderRoots {
			evict := api.finderRoots[0]
			api.finderRoots = api.finderRoots[1:]
			if err := api.finder.RemoveRoot(evict); err != nil {
				return nil, xerrors.Errorf("evict root %q: %w", evict, err)
			}
		}
		if err := api.finder.AddRoot(ctx, root); err != nil {
			return nil, xerrors.Errorf("index root %q: %w", root, err)
		}
		api.finderRoots = append(api.finderRoots, root)
	}

	opts := filefinder.DefaultSearchOptions()
	opts.Limit = limit
	// The engine searches every indexed root. Over-fetch when other
	// roots are indexed so their matches do not crowd out the
	// requested ones.
	if len(api.finderRoots) > len(roots) {
		opts.Limit = limit * maxFinderRoots
	}
	found, err := api.finder.Search(ctx, q, opts)
	if err != nil {
		return nil, xerrors.Errorf("search: %w", err)
	}

	results := make([]workspacesdk.FindFilesResult, 0, min(len(found), limit))
	seen := make(map[string]struct{}, len(found))
	for _, f := range found {
		if len(results) >= limit {
			break
		}
		if !slices.Contains(roots, f.Root) {
			continue
		}
		absPath := filepath.Join(f.Root, filepath.FromSlash(f.Path))
		if _, ok := seen[absPath]; ok {
			continue
		}
		seen[absPath] = struct{}{}
		results = append(results, workspacesdk.FindFilesResult{
			AbsolutePath: absPath,
			Path:         f.Path,
			Root:         f.Root,
			IsDir:        f.IsDir,
			Score:        f.Score,
		})
	}
	return results, nil
}
