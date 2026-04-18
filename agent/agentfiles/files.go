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

	"github.com/aymanbagabas/go-udiff"
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
	// origPath is the caller-supplied path, pre-symlink-resolution.
	// Used for response labels so the caller can match responses to
	// their original requests.
	origPath string
	// path is the symlink-resolved path; what actually gets written.
	path string
	// oldContent is the file content before edits were applied. Used
	// for diff computation when the request asked for diffs.
	oldContent string
	// content is the file content after all edits.
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

	// Duplicate entries both read the same file and race to write;
	// the first entry's edits are silently lost. Resolve symlinks
	// before comparing so two paths that alias the same real file
	// (e.g. one via a symlink, one direct) don't slip past as
	// distinct keys. prepareFileEdit resolves the path again for
	// its own use; the double lstat cost is cheap compared to the
	// data-loss risk of silent aliasing.
	type seenEntry struct {
		caller string
	}
	seenPaths := make(map[string]seenEntry, len(req.Files))
	for _, f := range req.Files {
		// On resolve error, use the raw path; phase 1 surfaces
		// the error with its proper status code.
		key := f.Path
		if resolved, err := api.resolvePath(f.Path); err == nil {
			key = resolved
		}
		if prev, dup := seenPaths[key]; dup {
			msg := fmt.Sprintf("duplicate file path %q: combine edits into a single entry's \"edits\" list", f.Path)
			if prev.caller != f.Path {
				msg = fmt.Sprintf("duplicate file path %q aliases %q (same real file): combine edits into a single entry's \"edits\" list", f.Path, prev.caller)
			}
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: msg,
			})
			return
		}
		seenPaths[key] = seenEntry{caller: f.Path}
	}

	// Phase 1: compute all edits in memory. If any file fails
	// (bad path, search miss, permission error), bail before
	// writing anything.
	var pending []pendingEdit
	var combinedErr error
	status := http.StatusOK
	for _, edit := range req.Files {
		s, p, err := api.prepareFileEdit(edit.Path, edit.Edits)
		if s > status {
			status = s
		}
		if err != nil {
			combinedErr = errors.Join(combinedErr, err)
		}
		if p != nil {
			pending = append(pending, *p)
		}
	}

	if combinedErr != nil {
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: combinedErr.Error(),
		})
		return
	}

	// Phase 2: write all files via atomicWrite. A failure here
	// (e.g. disk full) can leave earlier files committed. True
	// cross-file atomicity would require filesystem transactions.
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

	resp := workspacesdk.FileEditResponse{}
	if req.IncludeDiff {
		resp.Files = make([]workspacesdk.FileEditResult, 0, len(pending))
		for _, p := range pending {
			// udiff.Unified calls log.Fatalf on its internal error,
			// which would kill the agent process. Route through
			// Lines + ToUnified so a library bug yields an empty
			// diff plus a log line instead.
			edits := udiff.Lines(p.oldContent, p.content)
			diff, err := udiff.ToUnified(p.origPath, p.origPath, p.oldContent, edits, udiff.DefaultContextLines)
			if err != nil {
				api.logger.Warn(ctx, "unified diff computation failed",
					slog.F("path", p.origPath),
					slog.Error(err))
				diff = ""
			}
			resp.Files = append(resp.Files, workspacesdk.FileEditResult{
				Path: p.origPath,
				Diff: diff,
			})
		}
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
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
	origPath := path
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
	oldContent := content

	for _, edit := range edits {
		var err error
		content, err = fuzzyReplace(content, edit)
		if err != nil {
			return http.StatusBadRequest, nil, xerrors.Errorf("edit %s: %w", path, err)
		}
	}

	return 0, &pendingEdit{
		origPath:   origPath,
		path:       path,
		oldContent: oldContent,
		content:    content,
		mode:       stat.Mode(),
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

// splitEnding separates a line produced by strings.SplitAfter(s,
// "\n") into its content bytes and its line ending. The ending is
// one of "\r\n", "\n", or "" (the last slice when the input lacks a
// trailing newline).
func splitEnding(line string) (content, ending string) {
	if strings.HasSuffix(line, "\r\n") {
		return line[:len(line)-2], "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return line[:len(line)-1], "\n"
	}
	return line, ""
}

// endingsMatch decides whether two line endings may pair up during
// fuzzy matching. Identical endings always match. "\n" and "\r\n"
// interchange so LLMs can send LF searches against CRLF content.
// An empty ending (EOF, no terminator) acts as a wildcard and
// matches any ending, which lets the splice later substitute the
// file's actual ending in place of a missing one.
func endingsMatch(a, b string) bool {
	// Wildcard: empty ending matches any ending at the matching
	// phase. Only valid here, not at the splice phase.
	if a == "" || b == "" {
		return true
	}
	if a == b {
		return true
	}
	return isNewlineEnding(a) && isNewlineEnding(b)
}

// isNewlineEnding reports whether s is one of the newline-class
// endings: "\n" or "\r\n". Shared primitive for endingsMatch
// (matching phase) and endingShapeEqual (splice phase) so a new
// ending class added in one predicate can't silently diverge from
// the other.
func isNewlineEnding(s string) bool {
	return s == "\n" || s == "\r\n"
}

// internalLineEnding returns the shared line ending used across
// lines. An unterminated last line (EOF-no-newline) is excluded.
// Returns ("", false) if any non-last line has no ending, or if
// endings disagree.
func internalLineEnding(lines []string) (string, bool) {
	if len(lines) < 2 {
		return "", false
	}
	var want string
	for i, l := range lines {
		isLast := i == len(lines)-1
		_, e := splitEnding(l)
		if isLast && e == "" {
			continue
		}
		if e == "" {
			return "", false
		}
		if want == "" {
			want = e
			continue
		}
		if e != want {
			return "", false
		}
	}
	return want, want != ""
}

// dominantFileEnding returns CRLF if CRLF endings outnumber LF in
// contentLines, LF otherwise (including ties and ending-less files).
func dominantFileEnding(contentLines []string) string {
	var crlf, lf int
	for _, l := range contentLines {
		switch {
		case strings.HasSuffix(l, "\r\n"):
			crlf++
		case strings.HasSuffix(l, "\n"):
			lf++
		}
	}
	if crlf > lf {
		return "\r\n"
	}
	return "\n"
}

// atNoNewlineEOF reports whether the matched region ends at a
// file that lacks a trailing newline. True when no non-empty lines
// follow the match and the last matched line has no ending.
func atNoNewlineEOF(contentLines []string, end int) bool {
	if end == 0 {
		return false
	}
	if end < len(contentLines) {
		// Anything non-empty after the match disqualifies.
		for _, l := range contentLines[end:] {
			if l != "" {
				return false
			}
		}
	}
	// Last matched content line must itself have no ending.
	_, e := splitEnding(contentLines[end-1])
	return e == ""
}

// leadOnly returns the leading whitespace of line (spaces and
// tabs only), excluding the ending.
func leadOnly(line string) string {
	//nolint:dogsled // splitLineParts is the shared decomposer; other parts are genuinely unused here.
	lead, _, _, _ := splitLineParts(line)
	return lead
}

// alignSearchReplace returns the count of leading and trailing
// lines that match between searchLines and repLines under
// TrimSpace equality. Between the prefix and suffix ranges lies
// the middle: inserted, deleted, or rewritten lines. TrimSpace
// matches what pass 3 uses for matching, so pair identification
// stays consistent with how the region was found.
func alignSearchReplace(searchLines, repLines []string) (prefix, suffix int) {
	eq := func(a, b string) bool {
		aContent, _ := splitEnding(a)
		bContent, _ := splitEnding(b)
		return strings.TrimSpace(aContent) == strings.TrimSpace(bContent)
	}
	maxPrefix := len(searchLines)
	if len(repLines) < maxPrefix {
		maxPrefix = len(repLines)
	}
	for prefix < maxPrefix && eq(searchLines[prefix], repLines[prefix]) {
		prefix++
	}
	// Suffix must not overlap prefix on either side.
	maxSuffix := maxPrefix - prefix
	for suffix < maxSuffix &&
		eq(searchLines[len(searchLines)-1-suffix], repLines[len(repLines)-1-suffix]) {
		suffix++
	}
	return prefix, suffix
}

// detectIndentUnit scans leading whitespace across the given lines
// and returns the smallest consistent indentation unit (one tab, or
// N spaces where N is the GCD of observed non-zero lead lengths).
// Returns ("", false) when no useful unit can be detected: no lines
// have indent, indents mix tabs and spaces, or the GCD is zero.
//
// Tabs take priority: any tab-indented line forces unit="\t" and any
// space-only indent on another line marks the sample as mixed.
func detectIndentUnit(lines []string) (string, bool) {
	sawTab := false
	sawSpace := false
	var spaceGCD int
	for _, l := range lines {
		lead, mid, _, _ := splitLineParts(l)
		// Skip body-less lines: a blank line or a line with only
		// trailing whitespace has no indent signal. Otherwise a
		// 2sp whitespace-only line on a 4sp file would corrupt
		// the GCD down to 2sp and emit the wrong unit.
		if lead == "" || mid == "" {
			continue
		}
		switch {
		case strings.HasPrefix(lead, "\t") && !strings.ContainsAny(lead, " "):
			sawTab = true
		case !strings.ContainsAny(lead, "\t"):
			sawSpace = true
			if spaceGCD == 0 {
				spaceGCD = len(lead)
			} else {
				spaceGCD = indentGCD(spaceGCD, len(lead))
			}
		default:
			// Mixed tab+space in a single lead; bail.
			return "", false
		}
	}
	if sawTab && sawSpace {
		return "", false
	}
	if sawTab {
		return "\t", true
	}
	if spaceGCD > 0 {
		return strings.Repeat(" ", spaceGCD), true
	}
	return "", false
}

// indentGCD returns the greatest common divisor of a and b. Used
// only by detectIndentUnit on positive space-lead lengths.
func indentGCD(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// translateIndentLevel returns the file-side lead for an inserted
// splice line by translating the caller's indent level. rLead is
// the inserted replacement line's lead, sLead is the reference
// search line's lead (the pair the splice would have inherited
// from), cLead is the matched content's lead at that same
// reference slot. Returns ("", false) when any of the leads are
// not clean multiples of their respective units.
func translateIndentLevel(rLead, sLead, cLead, searchUnit, fileUnit string) (string, bool) {
	repLevel, ok := indentLevel(rLead, searchUnit)
	if !ok {
		return "", false
	}
	searchBase, ok := indentLevel(sLead, searchUnit)
	if !ok {
		return "", false
	}
	fileBase, ok := indentLevel(cLead, fileUnit)
	if !ok {
		return "", false
	}
	targetLevel := fileBase + (repLevel - searchBase)
	if targetLevel < 0 {
		return "", false
	}
	return strings.Repeat(fileUnit, targetLevel), true
}

// indentLevel returns len(lead) / len(unit) when lead is a clean
// multiple of unit. Returns (0, false) when lead doesn't divide
// evenly by unit. Callers must ensure unit is non-empty;
// detectIndentUnit's second return gates this.
func indentLevel(lead, unit string) (int, bool) {
	if len(lead)%len(unit) != 0 {
		return 0, false
	}
	// Verify the lead is actually composed of repetitions of unit.
	if strings.Repeat(unit, len(lead)/len(unit)) != lead {
		return 0, false
	}
	return len(lead) / len(unit), true
}

// non-last line's ending replaced by ending; the last line keeps
// its original ending. Used before pass 1 splicing to normalize
// the replacement to the file's ending style.
func rewriteInternalEnding(lines []string, ending string) string {
	var b strings.Builder
	for i, l := range lines {
		body, e := splitEnding(l)
		_, _ = b.WriteString(body)
		isLast := i == len(lines)-1
		switch {
		case isLast:
			_, _ = b.WriteString(e)
		case e == "":
			// Non-last line without ending is only legal at EOF;
			// leave the caller's shape alone.
		default:
			_, _ = b.WriteString(ending)
		}
	}
	return b.String()
}

// splitLineParts decomposes a line into its leading whitespace
// (spaces and tabs only), middle body, trailing whitespace
// (spaces and tabs only), and line ending. Used by the fuzzy
// splice to substitute the file's whitespace at each position
// when search and replace agree on what that position should be.
func splitLineParts(line string) (lead, middle, trail, ending string) {
	body, ending := splitEnding(line)
	i := 0
	for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
		i++
	}
	lead = body[:i]
	rest := body[i:]
	j := len(rest)
	for j > 0 && (rest[j-1] == ' ' || rest[j-1] == '\t') {
		j--
	}
	middle = rest[:j]
	trail = rest[j:]
	return lead, middle, trail, ending
}

// endingShapeEqual reports whether two line endings occupy the
// same "position class" for the splice substitution: both empty,
// or both in the newline class ({"\n", "\r\n"}). When this is
// true and the pair matched during matching, the splice uses the
// file's ending. When false, the splice keeps the replacement's
// ending verbatim (the caller is signaling an intentional fold
// or split). Unlike endingsMatch, empty is not a wildcard here:
// the splice phase needs a strict "same class" test so interior
// lines don't silently pick up a missing EOF terminator from the
// reference content.
func endingShapeEqual(a, b string) bool {
	if a == b {
		return true
	}
	return isNewlineEnding(a) && isNewlineEnding(b)
}

// buildReplacementLines emits the splice for a fuzzy match by
// per-position substitution at leading-ws, body, trailing-ws, and
// ending. Search and replace agreement at a position -> file's
// bytes win; disagreement -> replacement's bytes are spliced.
// Extra replace lines past the matched region reference the last
// search/content line.
//
// Carve-outs on "file wins on agreement":
//   - Empty replacement body: emit the replacement's whitespace
//     verbatim so a body-less line doesn't materialize whitespace.
//   - Reference content line has no ending and this isn't the
//     final replacement line: keep the replacement's newline so a
//     multi-line splice at EOF doesn't collapse.
//   - Inserted lines (no paired search line) try level-aware
//     indent translation: if we can detect both the caller's
//     search_unit and the file's fileUnit cleanly, the emitted
//     lead is fileUnit * (file_base + (rep_level - search_base)).
//     The caller's rep_level is computed from their own indent
//     style; output in the file's style so a 4sp LLM inserting
//     into a 2sp file emits 2sp indent at the correct depth. If
//     detection fails (no indent info, mixed tabs+spaces, or
//     a non-unit multiple), fall back to inheriting cLead.
//
// forcedEnding (from internalLineEnding normalization) overrides
// interior endings; the final ending is forced too unless
// atNoNewlineEOF (preserving the file's no-terminator EOF).
// When atNoNewlineEOF is false and the final ending would still
// be empty, force a terminator so unmatched content doesn't
// concatenate onto the splice.
//
// len(matched) == len(searchLines) is the invariant; callers
// slice contentLines before invoking.
//
//nolint:revive // atNoNewlineEOF is a computed match property, not caller control coupling.
func buildReplacementLines(matched, searchLines []string, replace, forcedEnding string, atNoNewlineEOF bool) string {
	repLines := strings.SplitAfter(replace, "\n")
	// SplitAfter on a string ending in "\n" yields a trailing empty
	// element. Drop it so it doesn't pair with a phantom line.
	if len(repLines) > 0 && repLines[len(repLines)-1] == "" {
		repLines = repLines[:len(repLines)-1]
	}
	prefix, suffix := alignSearchReplace(searchLines, repLines)

	// Combine search and replace so a zero-width search still
	// informs the unit from the replacement's inserted depths.
	// Fallback for detection failure lives in the inserted branch.
	searchUnit, searchUnitOK := detectIndentUnit(append(append([]string(nil), searchLines...), repLines...))
	fileUnit, fileUnitOK := detectIndentUnit(matched)
	var b strings.Builder
	for i, rLine := range repLines {
		var refIdx int
		inserted := false
		searchMiddleLen := len(searchLines) - prefix - suffix
		switch {
		case i < prefix:
			refIdx = i
		case i >= len(repLines)-suffix:
			refIdx = i - (len(repLines) - len(searchLines))
		case i-prefix < searchMiddleLen:
			refIdx = prefix + (i - prefix)
		default:
			// Pure insertion: pick the reference content line by
			// the caller's indent signal. An inserted line whose
			// lead matches the suffix's first rep line belongs to
			// the suffix scope; one matching the prefix's last rep
			// line belongs to the prefix scope. Fall back to
			// suffix, then prefix, then i-clamped.
			inserted = true
			rLeadForI := leadOnly(rLine)
			switch {
			case prefix > 0 && suffix > 0:
				prefixRLead := leadOnly(repLines[prefix-1])
				suffixRLead := leadOnly(repLines[len(repLines)-suffix])
				switch {
				case rLeadForI == suffixRLead:
					refIdx = len(searchLines) - suffix
				case rLeadForI == prefixRLead:
					refIdx = prefix - 1
				default:
					refIdx = len(searchLines) - suffix
				}
			case suffix > 0:
				refIdx = len(searchLines) - suffix
			case prefix > 0:
				refIdx = prefix - 1
			default:
				refIdx = min(i, len(searchLines)-1)
			}
		}
		refContent := matched[refIdx]
		sLead, _, sTrail, sEnd := splitLineParts(searchLines[refIdx])
		rLead, rMid, rTrail, rEnd := splitLineParts(rLine)
		cLead, _, cTrail, cEnd := splitLineParts(refContent)

		lead := rLead
		trail := rTrail
		switch {
		case rMid == "":
			// Body-less: emit the replacement's whitespace verbatim.
		case inserted:
			// Translate the caller's indent level to the file's
			// unit; fall back to cLead when detection fails.
			lead = cLead
			if searchUnitOK && fileUnitOK {
				if translated, ok := translateIndentLevel(rLead, sLead, cLead, searchUnit, fileUnit); ok {
					lead = translated
				}
			}
		default:
			if sLead == rLead {
				lead = cLead
			}
			if sTrail == rTrail {
				trail = cTrail
			}
		}
		ending := rEnd
		if !inserted && endingShapeEqual(sEnd, rEnd) {
			ending = cEnd
			// Interior lines keep their newline when the reference
			// content has cEnd="" (no-EOL EOF); only the final
			// output line may inherit the empty ending.
			if cEnd == "" && i < len(repLines)-1 {
				ending = rEnd
			}
		}
		if inserted && i == len(repLines)-1 && atNoNewlineEOF {
			ending = ""
		}
		if forcedEnding != "" && (i < len(repLines)-1 || !atNoNewlineEOF) {
			ending = forcedEnding
		}
		if i == len(repLines)-1 && !atNoNewlineEOF && ending == "" {
			if forcedEnding != "" {
				ending = forcedEnding
			} else {
				ending = "\n"
			}
		}

		_, _ = b.WriteString(lead)
		_, _ = b.WriteString(rMid)
		_, _ = b.WriteString(trail)
		_, _ = b.WriteString(ending)
	}
	return b.String()
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
// When a fuzzy match is found (passes 2 or 3), buildReplacementLines
// emits the spliced output by per-position substitution at
// leading-whitespace, body, trailing-whitespace, and ending: where
// search and replace agree at a position, the file's bytes win. This
// preserves surrounding text (including indentation of untouched
// lines) while letting the caller drive deliberate rewrites of
// leading whitespace or endings.
func fuzzyReplace(content string, edit workspacesdk.FileEdit) (string, error) {
	search := edit.Search
	replace := edit.Replace

	// An empty search string has no meaningful interpretation: it
	// matches at every byte position, which means the caller has not
	// told us what they want to replace. Reject explicitly so
	// replace_all=true can't silently inject the replacement between
	// every byte.
	if search == "" {
		return "", xerrors.New("search string must not be empty; include the " +
			"text you want to match")
	}

	// Split up front so the ending-normalization rule can inspect
	// all three before any matching pass.
	contentLines := strings.SplitAfter(content, "\n")
	searchLines := strings.SplitAfter(search, "\n")
	// A trailing newline in the search produces an empty final element
	// from SplitAfter. Drop it so it doesn't interfere with line
	// matching.
	if len(searchLines) > 0 && searchLines[len(searchLines)-1] == "" {
		searchLines = searchLines[:len(searchLines)-1]
	}
	replaceLines := strings.SplitAfter(replace, "\n")
	if len(replaceLines) > 0 && replaceLines[len(replaceLines)-1] == "" {
		replaceLines = replaceLines[:len(replaceLines)-1]
	}

	// Ending normalization. If replace has a consistent internal
	// ending, force every spliced interior line to the file's
	// dominant ending. If search also has a consistent internal
	// ending and it disagrees with replace's, the caller signaled
	// intent to rewrite endings; restrict the match to pass 1 so
	// CRLF/LF interchange at pass 2 can't silently bridge a search
	// that doesn't actually occur in the file.
	var forcedEnding string
	searchInternal, searchOK := internalLineEnding(searchLines)
	replaceInternal, replaceOK := internalLineEnding(replaceLines)
	if replaceOK {
		forcedEnding = dominantFileEnding(contentLines)
	}
	callerEndingIntent := searchOK && replaceOK && searchInternal != replaceInternal

	// Pass 1 - exact substring match. Normalize replace's interior
	// endings to the file's style unless the caller's search/replace
	// disagreement signaled intent to rewrite endings.
	pass1Replace := replace
	if forcedEnding != "" && !callerEndingIntent && replaceInternal != forcedEnding {
		pass1Replace = rewriteInternalEnding(replaceLines, forcedEnding)
	}
	if strings.Contains(content, search) {
		if edit.ReplaceAll {
			return strings.ReplaceAll(content, search, pass1Replace), nil
		}
		count := strings.Count(content, search)
		if count > 1 {
			return "", xerrors.Errorf("search string matches %d occurrences "+
				"(expected exactly 1). Include more surrounding "+
				"context to make the match unique, or set "+
				"replace_all to true", count)
		}
		// Exactly one match.
		return strings.Replace(content, search, pass1Replace, 1), nil
	}

	if callerEndingIntent {
		// Intent signaled but pass 1 missed; reject rather than let
		// pass 2's CRLF/LF interchange bridge a mismatched search.
		return "", xerrors.New("search string not found in file. Verify the search " +
			"string matches the file content exactly, including whitespace, " +
			"indentation, and line endings")
	}

	trimRight := func(a, b string) bool {
		aContent, aEnding := splitEnding(a)
		bContent, bEnding := splitEnding(b)
		return endingsMatch(aEnding, bEnding) &&
			strings.TrimRight(aContent, " \t") == strings.TrimRight(bContent, " \t")
	}
	trimAll := func(a, b string) bool {
		aContent, aEnding := splitEnding(a)
		bContent, bEnding := splitEnding(b)
		return endingsMatch(aEnding, bEnding) &&
			strings.TrimSpace(aContent) == strings.TrimSpace(bContent)
	}

	// Pass 2 – trim trailing whitespace on each line.
	if result, matched, err := fuzzyReplaceLines(contentLines, searchLines, replace, trimRight, edit.ReplaceAll, forcedEnding); matched {
		return result, err
	}

	// Pass 3 – trim all leading and trailing whitespace
	// (indentation-tolerant). The replacement is inserted verbatim;
	// callers must provide correctly indented replacement text.
	if result, matched, err := fuzzyReplaceLines(contentLines, searchLines, replace, trimAll, edit.ReplaceAll, forcedEnding); matched {
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
	forcedEnding string,
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
		var b strings.Builder
		for _, l := range contentLines[:start] {
			_, _ = b.WriteString(l)
		}
		_, _ = b.WriteString(buildReplacementLines(contentLines[start:end], searchLines, replace, forcedEnding, atNoNewlineEOF(contentLines, end)))
		for _, l := range contentLines[end:] {
			_, _ = b.WriteString(l)
		}
		return b.String(), true, nil
	}

	// Replace all: collect all match positions, then emit the
	// output forward, interleaving unmatched spans with spliced
	// replacements. Each match runs through the same per-position
	// splice as single-replace, using its own matched content
	// slice as the reference.
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

	var b strings.Builder
	prev := 0
	for _, m := range matches {
		for _, l := range contentLines[prev:m.start] {
			_, _ = b.WriteString(l)
		}
		_, _ = b.WriteString(buildReplacementLines(contentLines[m.start:m.end], searchLines, replace, forcedEnding, atNoNewlineEOF(contentLines, m.end)))
		prev = m.end
	}
	for _, l := range contentLines[prev:] {
		_, _ = b.WriteString(l)
	}
	return b.String(), true, nil
}
