package agentfiles

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
	"strings"
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

// ReadFileLinesResponse is the JSON response for the line-based file reader.
type ReadFileLinesResponse struct {
	// Success indicates whether the read was successful.
	Success bool `json:"success"`
	// FileSize is the original file size in bytes.
	FileSize int64 `json:"file_size,omitempty"`
	// TotalLines is the total number of lines in the file.
	TotalLines int `json:"total_lines,omitempty"`
	// LinesRead is the count of lines returned in this response.
	LinesRead int `json:"lines_read,omitempty"`
	// Content is the line-numbered file content.
	Content string `json:"content,omitempty"`
	// Error is the error message when success is false.
	Error string `json:"error,omitempty"`
}

type HTTPResponseCode = int

func (api *API) HandleReadFile(rw http.ResponseWriter, r *http.Request) {
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

	status, err := api.streamFile(ctx, rw, path, offset, limit)
	if err != nil {
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: err.Error(),
		})
		return
	}
}

func (api *API) streamFile(ctx context.Context, rw http.ResponseWriter, path string, offset, limit int64) (HTTPResponseCode, error) {
	if !filepath.IsAbs(path) {
		return http.StatusBadRequest, xerrors.Errorf("file path must be absolute: %q", path)
	}

	f, err := api.filesystem.Open(path)
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
		api.logger.Error(ctx, "workspace agent read file", slog.Error(err))
	}

	return 0, nil
}

func (api *API) HandleReadFileLines(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	parser := httpapi.NewQueryParamParser().RequiredNotEmpty("path")
	path := parser.String(query, "", "path")
	offset := parser.PositiveInt64(query, 1, "offset")
	limit := parser.PositiveInt64(query, 0, "limit")
	maxFileSize := parser.PositiveInt64(query, workspacesdk.DefaultMaxFileSize, "max_file_size")
	maxLineBytes := parser.PositiveInt64(query, workspacesdk.DefaultMaxLineBytes, "max_line_bytes")
	maxResponseLines := parser.PositiveInt64(query, workspacesdk.DefaultMaxResponseLines, "max_response_lines")
	maxResponseBytes := parser.PositiveInt64(query, workspacesdk.DefaultMaxResponseBytes, "max_response_bytes")
	parser.ErrorExcessParams(query)
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	resp := api.readFileLines(ctx, path, offset, limit, workspacesdk.ReadFileLinesLimits{
		MaxFileSize:      maxFileSize,
		MaxLineBytes:     int(maxLineBytes),
		MaxResponseLines: int(maxResponseLines),
		MaxResponseBytes: int(maxResponseBytes),
	})
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func (api *API) readFileLines(_ context.Context, path string, offset, limit int64, limits workspacesdk.ReadFileLinesLimits) ReadFileLinesResponse {
	errResp := func(msg string) ReadFileLinesResponse {
		return ReadFileLinesResponse{Success: false, Error: msg}
	}

	if !filepath.IsAbs(path) {
		return errResp(fmt.Sprintf("file path must be absolute: %q", path))
	}

	f, err := api.filesystem.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errResp(fmt.Sprintf("file does not exist: %s", path))
		}
		if errors.Is(err, os.ErrPermission) {
			return errResp(fmt.Sprintf("permission denied: %s", path))
		}
		return errResp(fmt.Sprintf("open file: %s", err))
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return errResp(fmt.Sprintf("stat file: %s", err))
	}

	if stat.IsDir() {
		return errResp(fmt.Sprintf("not a file: %s", path))
	}

	fileSize := stat.Size()
	if fileSize > limits.MaxFileSize {
		return errResp(fmt.Sprintf(
			"file is %d bytes which exceeds the maximum of %d bytes. Use grep, sed, or awk to extract the content you need, or use offset and limit to read a portion.",
			fileSize, limits.MaxFileSize,
		))
	}

	// Read the entire file (up to MaxFileSize).
	data, err := io.ReadAll(f)
	if err != nil {
		return errResp(fmt.Sprintf("read file: %s", err))
	}

	// Split into lines.
	content := string(data)
	// Handle empty file.
	if content == "" {
		return ReadFileLinesResponse{
			Success:    true,
			FileSize:   fileSize,
			TotalLines: 0,
			LinesRead:  0,
			Content:    "",
		}
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	// offset is 1-based line number.
	if offset < 1 {
		offset = 1
	}
	if offset > int64(totalLines) {
		return errResp(fmt.Sprintf(
			"offset %d is beyond the file length of %d lines",
			offset, totalLines,
		))
	}

	// Default limit.
	if limit <= 0 {
		limit = int64(limits.MaxResponseLines)
	}

	startIdx := int(offset - 1) // convert to 0-based
	endIdx := startIdx + int(limit)
	if endIdx > totalLines {
		endIdx = totalLines
	}

	var numbered []string
	totalBytesAccumulated := 0

	for i := startIdx; i < endIdx; i++ {
		line := lines[i]

		// Per-line truncation.
		if len(line) > limits.MaxLineBytes {
			line = line[:limits.MaxLineBytes] + "... [truncated]"
		}

		// Format with 1-based line number.
		numberedLine := fmt.Sprintf("%d\t%s", i+1, line)
		lineBytes := len(numberedLine)

		// Check total byte budget.
		newTotal := totalBytesAccumulated + lineBytes
		if len(numbered) > 0 {
			newTotal++ // account for \n joiner
		}
		if newTotal > limits.MaxResponseBytes {
			return errResp(fmt.Sprintf(
				"output would exceed %d bytes. Read less at a time using offset and limit parameters.",
				limits.MaxResponseBytes,
			))
		}

		// Check line count.
		if len(numbered) >= limits.MaxResponseLines {
			return errResp(fmt.Sprintf(
				"output would exceed %d lines. Read less at a time using offset and limit parameters.",
				limits.MaxResponseLines,
			))
		}

		numbered = append(numbered, numberedLine)
		totalBytesAccumulated = newTotal
	}

	return ReadFileLinesResponse{
		Success:    true,
		FileSize:   fileSize,
		TotalLines: totalLines,
		LinesRead:  len(numbered),
		Content:    strings.Join(numbered, "\n"),
	}
}

func (api *API) HandleWriteFile(rw http.ResponseWriter, r *http.Request) {
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

	status, err := api.writeFile(ctx, r, path)
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

func (api *API) writeFile(ctx context.Context, r *http.Request, path string) (HTTPResponseCode, error) {
	if !filepath.IsAbs(path) {
		return http.StatusBadRequest, xerrors.Errorf("file path must be absolute: %q", path)
	}

	dir := filepath.Dir(path)
	err := api.filesystem.MkdirAll(dir, 0o755)
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

	f, err := api.filesystem.Create(path)
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
		api.logger.Error(ctx, "workspace agent write file", slog.Error(err))
	}

	return 0, nil
}

func (api *API) HandleEditFiles(rw http.ResponseWriter, r *http.Request) {
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
		s, err := api.editFile(r.Context(), edit.Path, edit.Edits)
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

func (api *API) editFile(ctx context.Context, path string, edits []workspacesdk.FileEdit) (int, error) {
	if path == "" {
		return http.StatusBadRequest, xerrors.New("\"path\" is required")
	}

	if !filepath.IsAbs(path) {
		return http.StatusBadRequest, xerrors.Errorf("file path must be absolute: %q", path)
	}

	if len(edits) == 0 {
		return http.StatusBadRequest, xerrors.New("must specify at least one edit")
	}

	f, err := api.filesystem.Open(path)
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
	tmpfile, err := afero.TempFile(api.filesystem, filepath.Dir(path), filepath.Base(path))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer tmpfile.Close()

	_, err = io.Copy(tmpfile, replace.Chain(f, transforms...))
	if err != nil {
		if rerr := api.filesystem.Remove(tmpfile.Name()); rerr != nil {
			api.logger.Warn(ctx, "unable to clean up temp file", slog.Error(rerr))
		}
		return http.StatusInternalServerError, xerrors.Errorf("edit %s: %w", path, err)
	}

	err = api.filesystem.Rename(tmpfile.Name(), path)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return 0, nil
}
