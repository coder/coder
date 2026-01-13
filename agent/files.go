package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/icholy/replace"
	"github.com/spf13/afero"
	"golang.org/x/text/transform"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type HTTPResponseCode = int

func (a *agent) HandleReadFile(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	parser := httpapi.NewQueryParamParser().RequiredNotEmpty("path")
	path := parser.String(query, "", "path")
	offset := parser.PositiveInt64(query, 0, "offset")
	limit := parser.PositiveInt64(query, 0, "limit")
	parser.ErrorExcessParams(query)
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	status, err := a.streamFile(ctx, rw, path, offset, limit)
	if err != nil {
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: err.Error(),
		})
		return
	}
}

func (a *agent) streamFile(ctx context.Context, rw http.ResponseWriter, path string, offset, limit int64) (HTTPResponseCode, error) {
	if !filepath.IsAbs(path) {
		return http.StatusBadRequest, xerrors.Errorf("file path must be absolute: %q", path)
	}

	f, err := a.filesystem.Open(path)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, os.ErrNotExist):
			status = http.StatusNotFound
		case errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		}
		return status, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	if stat.IsDir() {
		return http.StatusBadRequest, xerrors.Errorf("open %s: not a file", path)
	}

	size := stat.Size()
	if limit == 0 {
		limit = size
	}
	bytesRemaining := max(size-offset, 0)
	bytesToRead := min(bytesRemaining, limit)

	// Relying on just the file name for the mime type for now.
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	rw.Header().Set("Content-Type", mimeType)
	rw.Header().Set("Content-Length", strconv.FormatInt(bytesToRead, 10))
	rw.WriteHeader(http.StatusOK)

	reader := io.NewSectionReader(f, offset, bytesToRead)
	_, err = io.Copy(rw, reader)
	if err != nil && !errors.Is(err, io.EOF) && ctx.Err() == nil {
		a.logger.Error(ctx, "workspace agent read file", slog.Error(err))
	}

	return 0, nil
}

func (a *agent) HandleWriteFile(rw http.ResponseWriter, r *http.Request) {
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

	status, err := a.writeFile(ctx, r, path)
	if err != nil {
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: fmt.Sprintf("Successfully wrote to %q", path),
	})
}

func (a *agent) writeFile(ctx context.Context, r *http.Request, path string) (HTTPResponseCode, error) {
	if !filepath.IsAbs(path) {
		return http.StatusBadRequest, xerrors.Errorf("file path must be absolute: %q", path)
	}

	dir := filepath.Dir(path)
	err := a.filesystem.MkdirAll(dir, 0o755)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		case errors.Is(err, syscall.ENOTDIR):
			status = http.StatusBadRequest
		}
		return status, err
	}

	f, err := a.filesystem.Create(path)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		case errors.Is(err, syscall.EISDIR):
			status = http.StatusBadRequest
		}
		return status, err
	}
	defer f.Close()

	_, err = io.Copy(f, r.Body)
	if err != nil && !errors.Is(err, io.EOF) && ctx.Err() == nil {
		a.logger.Error(ctx, "workspace agent write file", slog.Error(err))
	}

	return 0, nil
}

func (a *agent) HandleEditFiles(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req workspacesdk.FileEditRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	if len(req.Files) == 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "must specify at least one file",
		})
		return
	}

	var combinedErr error
	status := http.StatusOK
	for _, edit := range req.Files {
		s, err := a.editFile(r.Context(), edit.Path, edit.Edits)
		// Keep the highest response status, so 500 will be preferred over 400, etc.
		if s > status {
			status = s
		}
		if err != nil {
			combinedErr = errors.Join(combinedErr, err)
		}
	}

	if combinedErr != nil {
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: combinedErr.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: "Successfully edited file(s)",
	})
}

func (a *agent) editFile(ctx context.Context, path string, edits []workspacesdk.FileEdit) (int, error) {
	if path == "" {
		return http.StatusBadRequest, xerrors.New("\"path\" is required")
	}

	if !filepath.IsAbs(path) {
		return http.StatusBadRequest, xerrors.Errorf("file path must be absolute: %q", path)
	}

	if len(edits) == 0 {
		return http.StatusBadRequest, xerrors.New("must specify at least one edit")
	}

	f, err := a.filesystem.Open(path)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, os.ErrNotExist):
			status = http.StatusNotFound
		case errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		}
		return status, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	if stat.IsDir() {
		return http.StatusBadRequest, xerrors.Errorf("open %s: not a file", path)
	}

	transforms := make([]transform.Transformer, len(edits))
	for i, edit := range edits {
		transforms[i] = replace.String(edit.Search, edit.Replace)
	}

	// Create an adjacent file to ensure it will be on the same device and can be
	// moved atomically.
	tmpfile, err := afero.TempFile(a.filesystem, filepath.Dir(path), filepath.Base(path))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer tmpfile.Close()

	_, err = io.Copy(tmpfile, replace.Chain(f, transforms...))
	if err != nil {
		if rerr := a.filesystem.Remove(tmpfile.Name()); rerr != nil {
			a.logger.Warn(ctx, "unable to clean up temp file", slog.Error(rerr))
		}
		return http.StatusInternalServerError, xerrors.Errorf("edit %s: %w", path, err)
	}

	err = a.filesystem.Rename(tmpfile.Name(), path)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return 0, nil
}
