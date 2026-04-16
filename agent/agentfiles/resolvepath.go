package agentfiles

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// HandleResolvePath resolves the existing portion of an absolute path through
// any symlinks and returns the resulting path. Missing trailing components are
// preserved so callers can validate future writes against the real target.
func (api *API) HandleResolvePath(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	parser := httpapi.NewQueryParamParser().RequiredNotEmpty("path")
	path := parser.String(query, "", "path")
	parser.ErrorExcessParams(query)
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	resolved, err := api.resolvePath(path)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case !filepath.IsAbs(path):
			status = http.StatusBadRequest
		case errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{Message: err.Error()})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.ResolvePathResponse{
		ResolvedPath: resolved,
	})
}

// resolvePath resolves any symlinks in the existing portion of path while
// preserving missing trailing components.
func (api *API) resolvePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", xerrors.Errorf("file path must be absolute: %q", path)
	}

	path = filepath.Clean(path)

	lstater, hasLstat := api.filesystem.(afero.Lstater)
	if !hasLstat {
		return path, nil
	}
	targetReader, hasReadlink := api.filesystem.(afero.LinkReader)
	if !hasReadlink {
		return path, nil
	}

	const maxDepth = 40
	var resolve func(string, int) (string, error)
	resolve = func(path string, depth int) (string, error) {
		if depth > maxDepth {
			return "", xerrors.Errorf("too many levels of symlinks resolving %q", path)
		}

		info, _, err := lstater.LstatIfPossible(path)
		switch {
		case err == nil:
			if info.Mode()&os.ModeSymlink == 0 {
				dir := filepath.Dir(path)
				if dir == path {
					return path, nil
				}

				resolvedDir, err := resolve(dir, depth)
				if err != nil {
					return "", err
				}
				return filepath.Join(resolvedDir, filepath.Base(path)), nil
			}

			target, err := targetReader.ReadlinkIfPossible(path)
			if err != nil {
				return "", err
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			return resolve(filepath.Clean(target), depth+1)
		case errors.Is(err, os.ErrNotExist):
			dir := filepath.Dir(path)
			if dir == path {
				return path, nil
			}

			resolvedDir, err := resolve(dir, depth)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolvedDir, filepath.Base(path)), nil
		default:
			return "", err
		}
	}

	return resolve(path, 0)
}
