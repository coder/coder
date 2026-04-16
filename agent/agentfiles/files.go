package agentfiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentgit"
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

// pendingEdit holds the computed result of a file edit, ready to
// be written to disk.
type pendingEdit struct {
	path    string
	content string
	mode    os.FileMode
}

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

	// Track edited path for git watch.
	if api.pathStore != nil {
		if chatID, ancestorIDs, ok := agentgit.ExtractChatContext(r); ok {
			api.pathStore.AddPaths(append([]uuid.UUID{chatID}, ancestorIDs...), []string{path})
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
		Message: fmt.Sprintf("Successfully wrote to %q", path),
	})
}

func (api *API) writeFile(ctx context.Context, r *http.Request, path string) (HTTPResponseCode, error) {
	if !filepath.IsAbs(path) {
		return http.StatusBadRequest, xerrors.Errorf("file path must be absolute: %q", path)
	}

	resolved, err := api.resolvePath(path)
	if err != nil {
		return http.StatusInternalServerError, xerrors.Errorf("resolve symlink %q: %w", path, err)
	}
	path = resolved

	dir := filepath.Dir(path)
	err = api.filesystem.MkdirAll(dir, 0o755)
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

	// Check if the target already exists so we can preserve its
	// permissions on the temp file before rename.
	var mode *os.FileMode
	if stat, serr := api.filesystem.Stat(path); serr == nil {
		if stat.IsDir() {
			return http.StatusBadRequest, xerrors.Errorf("open %s: is a directory", path)
		}
		m := stat.Mode()
		mode = &m
	}

	return api.atomicWrite(ctx, path, mode, r.Body)
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

	// Classification pass: separate search/replace from ed-script
	// entries. Validate all entries upfront (before any writes).
	type edScriptEntry struct {
		path     string // Resolved (symlink-followed) path.
		origPath string // Original path from the request (for labels).
		script   string
		origMode os.FileMode
	}
	var (
		pending      []pendingEdit
		edEntries    []edScriptEntry
		combinedErr  error
		status       = http.StatusOK
		redAvailable bool
		redChecked   bool
		seenPaths    = make(map[string]struct{})
	)

	for _, edit := range req.Files {
		hasEdits := len(edit.Edits) > 0
		hasEdScript := edit.EdScript != ""

		switch {
		case hasEdits && hasEdScript:
			combinedErr = errors.Join(combinedErr,
				xerrors.Errorf("%s: cannot specify both edits and ed_script", edit.Path))
			if http.StatusBadRequest > status {
				status = http.StatusBadRequest
			}
		case !hasEdits && !hasEdScript:
			combinedErr = errors.Join(combinedErr,
				xerrors.Errorf("%s: must specify either edits or ed_script", edit.Path))
			if http.StatusBadRequest > status {
				status = http.StatusBadRequest
			}
		case hasEdScript:
			// Validate the ed-script entry upfront.
			if edit.Path == "" {
				combinedErr = errors.Join(combinedErr, xerrors.New("\"path\" is required"))
				if http.StatusBadRequest > status {
					status = http.StatusBadRequest
				}
				continue
			}
			if !filepath.IsAbs(edit.Path) {
				combinedErr = errors.Join(combinedErr,
					xerrors.Errorf("file path must be absolute: %q", edit.Path))
				if http.StatusBadRequest > status {
					status = http.StatusBadRequest
				}
				continue
			}
			resolved, err := api.resolvePath(edit.Path)
			if err != nil {
				combinedErr = errors.Join(combinedErr,
					xerrors.Errorf("resolve symlink %q: %w", edit.Path, err))
				if http.StatusInternalServerError > status {
					status = http.StatusInternalServerError
				}
				continue
			}
			// Verify the file exists and is not a directory.
			info, err := os.Stat(resolved)
			if err != nil {
				s := http.StatusInternalServerError
				switch {
				case errors.Is(err, os.ErrNotExist):
					s = http.StatusNotFound
				case errors.Is(err, os.ErrPermission):
					s = http.StatusForbidden
				}
				combinedErr = errors.Join(combinedErr, err)
				if s > status {
					status = s
				}
				continue
			}
			if info.IsDir() {
				combinedErr = errors.Join(combinedErr,
					xerrors.Errorf("open %s: not a file", resolved))
				if http.StatusBadRequest > status {
					status = http.StatusBadRequest
				}
				continue
			}
			// Check for red binary once per request.
			if !redChecked {
				if _, err := exec.LookPath("red"); err == nil {
					redAvailable = true
				}
				redChecked = true
			}
			if !redAvailable {
				combinedErr = errors.Join(combinedErr,
					xerrors.New("ed_script requires the 'ed' package to be installed (provides 'red'); install it with: apt-get install ed"))
				if http.StatusInternalServerError > status {
					status = http.StatusInternalServerError
				}
				continue
			}
			if _, dup := seenPaths[resolved]; dup {
				combinedErr = errors.Join(combinedErr,
					xerrors.Errorf("%s: duplicate file path in request", edit.Path))
				if http.StatusBadRequest > status {
					status = http.StatusBadRequest
				}
				continue
			}
			seenPaths[resolved] = struct{}{}
			edEntries = append(edEntries, edScriptEntry{
				path:     resolved,
				origPath: edit.Path,
				script:   edit.EdScript,
				origMode: info.Mode(),
			})
		default:
			// Search/replace path (existing logic).
			s, p, err := api.prepareFileEdit(edit.Path, edit.Edits)
			if s > status {
				status = s
			}
			if err != nil {
				combinedErr = errors.Join(combinedErr, err)
			}
			if p != nil {
				if _, dup := seenPaths[p.path]; dup {
					combinedErr = errors.Join(combinedErr,
						xerrors.Errorf("%s: duplicate file path in request", edit.Path))
					if http.StatusBadRequest > status {
						status = http.StatusBadRequest
					}
					continue
				}
				seenPaths[p.path] = struct{}{}
				pending = append(pending, *p)
			}
		}
	}

	if combinedErr != nil {
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: combinedErr.Error(),
		})
		return
	}

	var diffs []workspacesdk.FileEditDiff

	// Phase 2: write all search/replace files via atomicWrite.
	// A failure here (e.g. disk full) can leave earlier files
	// committed. True cross-file atomicity would require
	// filesystem transactions. The same applies to Phase 3
	// (ed-script entries).
	for _, p := range pending {
		mode := p.mode
		s, err := api.atomicWrite(ctx, p.path, &mode, strings.NewReader(p.content))
		if err != nil {
			httpapi.Write(ctx, rw, s, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
	}

	// Phase 3: process ed-script entries sequentially.
	for _, entry := range edEntries {
		diff, err := api.applyEdScript(ctx, entry.path, entry.origPath, entry.script, entry.origMode)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: err.Error(),
			})
			return
		}
		diffs = append(diffs, workspacesdk.FileEditDiff{
			Path: entry.origPath,
			Diff: diff,
		})
	}

	// Track edited paths for git watch.
	if api.pathStore != nil {
		if chatID, ancestorIDs, ok := agentgit.ExtractChatContext(r); ok {
			filePaths := make([]string, 0, len(req.Files))
			for _, f := range req.Files {
				filePaths = append(filePaths, f.Path)
			}
			api.pathStore.AddPaths(append([]uuid.UUID{chatID}, ancestorIDs...), filePaths)
		}
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.FileEditResponse{
		Diffs: diffs,
	})
}

// applyEdScript runs an ed script against a file atomically. It
// copies the file to an isolated temp directory, runs red against
// the copy, computes a unified diff, and renames the copy over
// the original. The isolated directory prevents ed w/r commands
// from reaching other files in the target's directory.
// Returns the diff string (empty if no changes) or an error.
func (api *API) applyEdScript(ctx context.Context, path, origPath, script string, origMode os.FileMode) (string, error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// Create an isolated temp directory so red cannot w/r other
	// files in the original file's directory.
	isoDir, err := os.MkdirTemp(dir, ".ed-*")
	if err != nil {
		return "", xerrors.Errorf("create isolated temp dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(isoDir); rmErr != nil {
			api.logger.Warn(ctx, "unable to clean up temp dir",
				slog.F("path", isoDir), slog.Error(rmErr))
		}
	}()

	tmpPath := filepath.Join(isoDir, base)

	// Copy original to temp file in the isolated directory.
	src, err := os.Open(path)
	if err != nil {
		return "", xerrors.Errorf("open %s: %w", path, err)
	}
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, origMode)
	if err != nil {
		_ = src.Close()
		return "", xerrors.Errorf("create temp file: %w", err)
	}
	_, err = io.Copy(tmpFile, src)
	_ = src.Close()
	_ = tmpFile.Close()
	if err != nil {
		return "", xerrors.Errorf("copy %s to temp: %w", path, err)
	}

	// Build the ed script: prepend H (verbose errors), append
	// a safety "." (closes any unterminated input mode), then
	// w (write) and q (quit).
	fullScript := "H\n" + script + "\n.\nw\nq\n"

	// Run red (restricted ed) in the isolated directory. red
	// restricts filenames to the current directory, and the
	// isolated directory contains only the target file copy.
	cmd := api.execer.CommandContext(ctx, "red", "-s", base)
	cmd.Dir = isoDir
	cmd.Stdin = strings.NewReader(fullScript)
	var combinedOut strings.Builder
	cmd.Stdout = &combinedOut
	cmd.Stderr = &combinedOut

	if err := cmd.Run(); err != nil {
		output := strings.TrimSpace(combinedOut.String())
		if output != "" {
			return "", xerrors.Errorf("ed script failed on %s: %s", path, output)
		}
		return "", xerrors.Errorf("ed script failed on %s: %w", path, err)
	}

	// Compute unified diff between original and edited temp.
	// Use --label with the full path to produce headers that
	// the frontend can match to files unambiguously.
	diffCmd := api.execer.CommandContext(ctx, "diff", "-u",
		"--label", "a/"+origPath, "--label", "b/"+origPath,
		path, tmpPath)
	diffOut, diffErr := diffCmd.CombinedOutput()
	// diff exits 0 = identical, 1 = different (expected),
	// 2 = trouble. In all failure cases the edit already
	// succeeded in the temp file, so log and continue
	// without a diff rather than discarding the edit.
	var diffStr string
	switch {
	case diffErr == nil:
		diffStr = string(diffOut)
	case diffCmd.ProcessState != nil && diffCmd.ProcessState.ExitCode() == 2:
		api.logger.Warn(ctx, "diff command failed",
			slog.F("path", path), slog.F("output", string(diffOut)))
	case diffCmd.ProcessState == nil:
		api.logger.Warn(ctx, "diff command failed to start",
			slog.F("path", path), slog.Error(diffErr))
	default:
		// Exit code 1 (files differ) is the expected case.
		diffStr = string(diffOut)
	}

	// Rename temp over original (atomic on same filesystem).
	if err := os.Rename(tmpPath, path); err != nil {
		return "", xerrors.Errorf("rename temp to %s: %w", path, err)
	}

	return diffStr, nil
}

// prepareFileEdit validates, reads, and computes edits for a single
// file without writing anything to disk.
func (api *API) prepareFileEdit(path string, edits []workspacesdk.FileEdit) (int, *pendingEdit, error) {
	if path == "" {
		return http.StatusBadRequest, nil, xerrors.New("\"path\" is required")
	}

	if !filepath.IsAbs(path) {
		return http.StatusBadRequest, nil, xerrors.Errorf("file path must be absolute: %q", path)
	}

	if len(edits) == 0 {
		return http.StatusBadRequest, nil, xerrors.New("must specify at least one edit")
	}

	resolved, err := api.resolvePath(path)
	if err != nil {
		return http.StatusInternalServerError, nil, xerrors.Errorf("resolve symlink %q: %w", path, err)
	}
	path = resolved

	f, err := api.filesystem.Open(path)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, os.ErrNotExist):
			status = http.StatusNotFound
		case errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		}
		return status, nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}

	if stat.IsDir() {
		return http.StatusBadRequest, nil, xerrors.Errorf("open %s: not a file", path)
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return http.StatusInternalServerError, nil, xerrors.Errorf("read %s: %w", path, err)
	}
	content := string(data)

	for _, edit := range edits {
		var err error
		content, err = fuzzyReplace(content, edit)
		if err != nil {
			return http.StatusBadRequest, nil, xerrors.Errorf("edit %s: %w", path, err)
		}
	}

	return 0, &pendingEdit{
		path:    path,
		content: content,
		mode:    stat.Mode(),
	}, nil
}

// atomicWrite writes content from r to path via a temp file in the
// same directory. If the target exists, its permissions are preserved.
// On failure the temp file is cleaned up and the original is
// untouched.
func (api *API) atomicWrite(ctx context.Context, path string, mode *os.FileMode, r io.Reader) (int, error) {
	dir := filepath.Dir(path)
	tmpName := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%s", filepath.Base(path), uuid.New().String()[:8]))

	tmpfile, err := api.filesystem.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrPermission) {
			status = http.StatusForbidden
		}
		return status, err
	}

	cleanup := func() {
		if err := api.filesystem.Remove(tmpName); err != nil {
			api.logger.Warn(ctx, "unable to clean up temp file", slog.Error(err))
		}
	}

	_, err = io.Copy(tmpfile, r)
	if err != nil {
		_ = tmpfile.Close()
		cleanup()
		return http.StatusInternalServerError, xerrors.Errorf("write %s: %w", path, err)
	}

	// Close before rename to flush buffered data and catch write
	// errors (e.g. delayed allocation failures).
	if err := tmpfile.Close(); err != nil {
		cleanup()
		return http.StatusInternalServerError, xerrors.Errorf("write %s: %w", path, err)
	}

	// Set permissions on the temp file before rename so there is
	// no window where the target has wrong permissions.
	if mode != nil {
		if err := api.filesystem.Chmod(tmpName, *mode); err != nil {
			api.logger.Warn(ctx, "unable to set file permissions",
				slog.F("path", path),
				slog.Error(err),
			)
		}
	}

	if err := api.filesystem.Rename(tmpName, path); err != nil {
		cleanup()
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrPermission) {
			status = http.StatusForbidden
		}
		return status, xerrors.Errorf("write %s: %w", path, err)
	}

	return 0, nil
}

// fuzzyReplace attempts to find `search` inside `content` and replace it
// with `replace`. It uses a cascading match strategy inspired by
// openai/codex's apply_patch:
//
//  1. Exact substring match (byte-for-byte).
//  2. Line-by-line match ignoring trailing whitespace on each line.
//  3. Line-by-line match ignoring all leading/trailing whitespace
//     (indentation-tolerant).
//
// When edit.ReplaceAll is false (the default), the search string must
// match exactly one location. If multiple matches are found, an error
// is returned asking the caller to include more context or set
// replace_all.
//
// When a fuzzy match is found (passes 2 or 3), the replacement is still
// applied at the byte offsets of the original content so that surrounding
// text (including indentation of untouched lines) is preserved.
func fuzzyReplace(content string, edit workspacesdk.FileEdit) (string, error) {
	search := edit.Search
	replace := edit.Replace

	// Pass 1 – exact substring match.
	if strings.Contains(content, search) {
		if edit.ReplaceAll {
			return strings.ReplaceAll(content, search, replace), nil
		}
		count := strings.Count(content, search)
		if count > 1 {
			return "", xerrors.Errorf("search string matches %d occurrences "+
				"(expected exactly 1). Include more surrounding "+
				"context to make the match unique, or set "+
				"replace_all to true", count)
		}
		// Exactly one match.
		return strings.Replace(content, search, replace, 1), nil
	}

	// For line-level fuzzy matching we split both content and search
	// into lines.
	contentLines := strings.SplitAfter(content, "\n")
	searchLines := strings.SplitAfter(search, "\n")

	// A trailing newline in the search produces an empty final element
	// from SplitAfter. Drop it so it doesn't interfere with line
	// matching.
	if len(searchLines) > 0 && searchLines[len(searchLines)-1] == "" {
		searchLines = searchLines[:len(searchLines)-1]
	}

	trimRight := func(a, b string) bool {
		return strings.TrimRight(a, " \t\r\n") == strings.TrimRight(b, " \t\r\n")
	}
	trimAll := func(a, b string) bool {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}

	// Pass 2 – trim trailing whitespace on each line.
	if result, matched, err := fuzzyReplaceLines(contentLines, searchLines, replace, trimRight, edit.ReplaceAll); matched {
		return result, err
	}

	// Pass 3 – trim all leading and trailing whitespace
	// (indentation-tolerant). The replacement is inserted verbatim;
	// callers must provide correctly indented replacement text.
	if result, matched, err := fuzzyReplaceLines(contentLines, searchLines, replace, trimAll, edit.ReplaceAll); matched {
		return result, err
	}

	return "", xerrors.New("search string not found in file. Verify the search " +
		"string matches the file content exactly, including whitespace " +
		"and indentation")
}

// seekLines scans contentLines looking for a contiguous subsequence that matches
// searchLines according to the provided `eq` function. It returns the start and
// end (exclusive) indices into contentLines of the match.
func seekLines(contentLines, searchLines []string, eq func(a, b string) bool) (start, end int, ok bool) {
	if len(searchLines) == 0 {
		return 0, 0, true
	}
	if len(searchLines) > len(contentLines) {
		return 0, 0, false
	}
outer:
	for i := 0; i <= len(contentLines)-len(searchLines); i++ {
		for j, sLine := range searchLines {
			if !eq(contentLines[i+j], sLine) {
				continue outer
			}
		}
		return i, i + len(searchLines), true
	}
	return 0, 0, false
}

// countLineMatches counts how many non-overlapping contiguous
// subsequences of contentLines match searchLines according to eq.
func countLineMatches(contentLines, searchLines []string, eq func(a, b string) bool) int {
	count := 0
	if len(searchLines) == 0 || len(searchLines) > len(contentLines) {
		return count
	}
outer:
	for i := 0; i <= len(contentLines)-len(searchLines); i++ {
		for j, sLine := range searchLines {
			if !eq(contentLines[i+j], sLine) {
				continue outer
			}
		}
		count++
		i += len(searchLines) - 1 // skip past this match
	}
	return count
}

// spliceLines replaces contentLines[start:end] with replacement text, returning
// the full content as a single string.
func spliceLines(contentLines []string, start, end int, replacement string) string {
	var b strings.Builder
	for _, l := range contentLines[:start] {
		_, _ = b.WriteString(l)
	}
	_, _ = b.WriteString(replacement)
	for _, l := range contentLines[end:] {
		_, _ = b.WriteString(l)
	}
	return b.String()
}

// fuzzyReplaceLines handles fuzzy matching passes (2 and 3) for
// fuzzyReplace. When replaceAll is false and there are multiple
// matches, an error is returned. When replaceAll is true, all
// non-overlapping matches are replaced.
//
// Returns (result, true, nil) on success, ("", false, nil) when
// searchLines don't match at all, or ("", true, err) when the match
// is ambiguous.
//
//nolint:revive // replaceAll is a direct pass-through of the user's flag, not a control coupling.
func fuzzyReplaceLines(
	contentLines, searchLines []string,
	replace string,
	eq func(a, b string) bool,
	replaceAll bool,
) (string, bool, error) {
	start, end, ok := seekLines(contentLines, searchLines, eq)
	if !ok {
		return "", false, nil
	}

	if !replaceAll {
		if count := countLineMatches(contentLines, searchLines, eq); count > 1 {
			return "", true, xerrors.Errorf("search string matches %d occurrences "+
				"(expected exactly 1). Include more surrounding "+
				"context to make the match unique, or set "+
				"replace_all to true", count)
		}
		return spliceLines(contentLines, start, end, replace), true, nil
	}

	// Replace all: collect all match positions, then apply from last
	// to first to preserve indices.
	type lineMatch struct{ start, end int }
	var matches []lineMatch
	for i := 0; i <= len(contentLines)-len(searchLines); {
		found := true
		for j, sLine := range searchLines {
			if !eq(contentLines[i+j], sLine) {
				found = false
				break
			}
		}
		if found {
			matches = append(matches, lineMatch{i, i + len(searchLines)})
			i += len(searchLines) // skip past this match
		} else {
			i++
		}
	}

	// Apply replacements from last to first.
	repLines := strings.SplitAfter(replace, "\n")
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		newLines := make([]string, 0, m.start+len(repLines)+(len(contentLines)-m.end))
		newLines = append(newLines, contentLines[:m.start]...)
		newLines = append(newLines, repLines...)
		newLines = append(newLines, contentLines[m.end:]...)
		contentLines = newLines
	}

	var b strings.Builder
	for _, l := range contentLines {
		_, _ = b.WriteString(l)
	}
	return b.String(), true, nil
}
