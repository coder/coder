package chattool

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	pathpkg "path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/agentmemory"
	"github.com/coder/coder/v2/coderd/database"
)

const (
	memoryPageSize = 25
	// memoryHeadlineStartMarker is injected by PostgreSQL ts_headline around
	// matching terms. It is not part of the stored memory content.
	memoryHeadlineStartMarker = "<memory-hit>"
)

// MemoryToolsOptions configures the user-scoped memory tools.
type MemoryToolsOptions struct {
	DB      database.Store
	UserID  uuid.UUID
	Context func(context.Context) context.Context
}

func (o MemoryToolsOptions) queryContext(ctx context.Context) context.Context {
	if o.Context != nil {
		return o.Context(ctx)
	}
	return ctx
}

// MemoryReadTools returns the read-only tools that are safe for plan turns
// and delegated agents.
func MemoryReadTools(opts MemoryToolsOptions) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		readMemoryTool(opts),
		searchMemoriesTool(opts),
		listMemoriesTool(opts),
	}
}

// MemoryWriteTools returns the tools that mutate user memories.
func MemoryWriteTools(opts MemoryToolsOptions) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		writeMemoryTool(opts),
		editMemoryTool(opts),
	}
}

type readMemoryArgs struct {
	Path string `json:"path" description:"Absolute memory path ending in .md"`
}

type writeMemoryArgs struct {
	Path    string `json:"path" description:"Absolute memory path ending in .md"`
	Content string `json:"content" description:"Markdown content to store"`
}

type editMemoryArgs struct {
	Path  string       `json:"path" description:"Absolute memory path ending in .md"`
	Edits []memoryEdit `json:"edits" description:"Atomic hashline edit operations"`
}

type memoryEdit struct {
	Op          string `json:"op" description:"One of set_line, replace_range, insert_before, insert_after, delete_line, or delete_range"`
	Anchor      string `json:"anchor,omitempty" description:"LINE_NUMBER:HASH anchor for a single line"`
	StartAnchor string `json:"start_anchor,omitempty" description:"Inclusive first LINE_NUMBER:HASH anchor"`
	EndAnchor   string `json:"end_anchor,omitempty" description:"Inclusive last LINE_NUMBER:HASH anchor"`
	NewText     string `json:"new_text,omitempty" description:"Replacement or inserted text"`
}

type searchMemoriesArgs struct {
	Keywords string   `json:"keywords" description:"Space-separated keywords. Example: postgres postgresql database workspace"`
	Paths    []string `json:"paths" description:"Optional absolute path globs; an empty array searches all paths"`
	Offset   *int32   `json:"offset,omitempty" description:"Zero-based result offset"`
}

type listMemoriesArgs struct {
	Directory string `json:"directory" description:"Absolute virtual directory glob"`
	Offset    *int32 `json:"offset,omitempty" description:"Zero-based result offset"`
}

type memoryView struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type memoryExcerpt struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
}

type memorySearchMatch struct {
	Path      string          `json:"path"`
	Name      string          `json:"name"`
	Rank      float32         `json:"rank"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Excerpts  []memoryExcerpt `json:"excerpts"`
}

type memoryListEntry struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	SizeBytes int    `json:"size_bytes"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func readMemoryTool(opts MemoryToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_memory",
		"Read a user memory by its absolute path. The Markdown content is returned with hashline anchors for safe edits.",
		func(ctx context.Context, args readMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if err := validateMemoryPath(args.Path); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			memory, err := opts.DB.GetAgentMemoryByUserIDAndPath(opts.queryContext(ctx), database.GetAgentMemoryByUserIDAndPathParams{
				UserID: opts.UserID,
				Path:   args.Path,
			})
			if errors.Is(err, sql.ErrNoRows) {
				return fantasy.NewTextErrorResponse("memory not found: " + args.Path), nil
			}
			if err != nil {
				return fantasy.NewTextErrorResponse("read memory: " + err.Error()), nil
			}
			return marshalToolResponse(memoryToView(memory)), nil
		},
	)
}

func writeMemoryTool(opts MemoryToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"write_memory",
		"Create a new user memory. The path must not already exist; use edit_memory to change an existing memory.",
		func(ctx context.Context, args writeMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if err := validateMemoryPath(args.Path); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if len(args.Content) > agentmemory.MaxContentBytes {
				return fantasy.NewTextErrorResponse("memory content exceeds 65536 bytes"), nil
			}
			memory, err := opts.DB.InsertAgentMemory(opts.queryContext(ctx), database.InsertAgentMemoryParams{
				ID:      uuid.New(),
				UserID:  opts.UserID,
				Path:    args.Path,
				Content: args.Content,
			})
			if database.IsUniqueViolation(err) {
				return fantasy.NewTextErrorResponse("memory already exists; read it and use edit_memory: " + args.Path), nil
			}
			if err != nil {
				return fantasy.NewTextErrorResponse("write memory: " + err.Error()), nil
			}
			return marshalToolResponse(memoryToView(memory)), nil
		},
	)
}

func editMemoryTool(opts MemoryToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"edit_memory",
		"Atomically edit a user memory using hashline anchors from read_memory. All anchors are checked against the current content before any edit is applied.",
		func(ctx context.Context, args editMemoryArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if err := validateMemoryPath(args.Path); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if len(args.Edits) == 0 {
				return fantasy.NewTextErrorResponse("edits is required"), nil
			}

			var updated database.AgentMemory
			err := opts.DB.InTx(func(tx database.Store) error {
				memory, err := tx.GetAgentMemoryByUserIDAndPathForUpdate(opts.queryContext(ctx), database.GetAgentMemoryByUserIDAndPathForUpdateParams{
					UserID: opts.UserID,
					Path:   args.Path,
				})
				if err != nil {
					return err
				}
				content, err := applyMemoryEdits(memory.Content, args.Edits)
				if err != nil {
					return err
				}
				if len(content) > agentmemory.MaxContentBytes {
					return xerrors.New("memory content exceeds 65536 bytes")
				}
				updated, err = tx.UpdateAgentMemoryContent(opts.queryContext(ctx), database.UpdateAgentMemoryContentParams{
					UserID:  opts.UserID,
					Path:    args.Path,
					Content: content,
				})
				return err
			}, nil)
			if errors.Is(err, sql.ErrNoRows) {
				return fantasy.NewTextErrorResponse("memory not found: " + args.Path), nil
			}
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return marshalToolResponse(memoryToView(updated)), nil
		},
	)
}

func searchMemoriesTool(opts MemoryToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"search_memories",
		`Search path names and Markdown content.

keywords are space-separated words. They are matched with OR, so any keyword may hit.
Prefer many individual related keywords over sentences.

Good example: "postgres postgresql database workspace pg db pgsql"
Bad example: "has the user ever mentioned using postgres"

paths are optional case-sensitive filesystem globs; an empty array searches all paths.`,
		func(ctx context.Context, args searchMemoriesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			query, err := memorySearchKeywordsQuery(args.Keywords)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			offset, err := memoryOffset(args.Offset)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			regexes := make([]string, 0, len(args.Paths))
			for _, pattern := range args.Paths {
				re, err := compileMemoryGlob(pattern)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				regexes = append(regexes, re)
			}
			rows, err := opts.DB.SearchAgentMemories(opts.queryContext(ctx), database.SearchAgentMemoriesParams{
				UserID:      opts.UserID,
				PathRegexes: regexes,
				OffsetValue: offset,
				Keywords:    query,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse("search memories: " + err.Error()), nil
			}
			hasMore := len(rows) > memoryPageSize
			if hasMore {
				rows = rows[:memoryPageSize]
			}
			matches := make([]memorySearchMatch, 0, len(rows))
			for _, row := range rows {
				matches = append(matches, memorySearchMatch{
					Path:      row.Path,
					Name:      pathpkg.Base(row.Path),
					Rank:      row.Rank,
					CreatedAt: row.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
					UpdatedAt: row.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
					Excerpts:  memoryExcerpts(row.Content, row.Headline),
				})
			}
			result := map[string]any{"matches": matches}
			if hasMore {
				result["next_offset"] = offset + memoryPageSize
			}
			return toolResponse(result), nil
		},
	)
}

func listMemoriesTool(opts MemoryToolsOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"list_memories",
		"Recursively list user memories beneath virtual directories matched by a case-sensitive filesystem glob.",
		func(ctx context.Context, args listMemoriesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			directoryRegex, err := compileMemoryDirectoryGlob(args.Directory)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			offset, err := memoryOffset(args.Offset)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			rows, err := opts.DB.ListAgentMemories(opts.queryContext(ctx), database.ListAgentMemoriesParams{
				UserID:         opts.UserID,
				DirectoryRegex: directoryRegex,
				OffsetValue:    offset,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse("list memories: " + err.Error()), nil
			}
			hasMore := len(rows) > memoryPageSize
			if hasMore {
				rows = rows[:memoryPageSize]
			}
			entries := make([]memoryListEntry, 0, len(rows))
			for _, row := range rows {
				entries = append(entries, memoryListEntry{
					Path:      row.Path,
					Name:      pathpkg.Base(row.Path),
					SizeBytes: int(row.SizeBytes),
					CreatedAt: row.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
					UpdatedAt: row.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
				})
			}
			result := map[string]any{"memories": entries}
			if hasMore {
				result["next_offset"] = offset + memoryPageSize
			}
			return toolResponse(result), nil
		},
	)
}

func memoryToView(memory database.AgentMemory) memoryView {
	return memoryView{
		Path:      memory.Path,
		Name:      pathpkg.Base(memory.Path),
		Content:   renderMemoryHashlines(memory.Content),
		CreatedAt: memory.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
		UpdatedAt: memory.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
	}
}

func memoryOffset(offset *int32) (int32, error) {
	if offset == nil {
		return 0, nil
	}
	if *offset < 0 {
		return 0, xerrors.New("offset must not be negative")
	}
	return *offset, nil
}

// memorySearchKeywordsQuery turns space-separated keywords into a
// websearch_to_tsquery string where any keyword may match.
func memorySearchKeywordsQuery(keywords string) (string, error) {
	tokens := strings.Fields(keywords)
	if len(tokens) == 0 {
		return "", xerrors.New("keywords is required")
	}
	return strings.Join(tokens, " OR "), nil
}

func validateMemoryPath(memoryPath string) error {
	return agentmemory.ValidatePath(memoryPath)
}

func compileMemoryGlob(pattern string) (string, error) {
	if pattern == "" {
		return "", xerrors.New("path glob is required")
	}
	if !utf8.ValidString(pattern) {
		return "", xerrors.New("path glob must be valid UTF-8")
	}
	if len(pattern) > agentmemory.MaxPathBytes {
		return "", xerrors.New("path glob exceeds 1024 bytes")
	}
	if !strings.HasPrefix(pattern, "/") || (len(pattern) > 1 && strings.HasSuffix(pattern, "/")) {
		return "", xerrors.New("path glob must be absolute and canonical")
	}
	for _, r := range pattern {
		if unicode.IsControl(r) {
			return "", xerrors.New("path glob must not contain control characters")
		}
	}

	segments := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	var out strings.Builder
	_ = out.WriteByte('^')
	if pattern == "/" {
		_, _ = out.WriteString("/$")
		return out.String(), nil
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", xerrors.New("path glob must be canonical")
		}
		if segment == "**" {
			_, _ = out.WriteString("(/[^/]+)*")
			continue
		}
		_ = out.WriteByte('/')
		for i := 0; i < len(segment); i++ {
			ch := segment[i]
			if ch == '\\' {
				if i+1 >= len(segment) {
					return "", xerrors.New("path glob has an incomplete escape")
				}
				i++
				writeMemoryRegexLiteral(&out, segment[i])
				continue
			}
			if ch == '*' {
				if i+1 < len(segment) && segment[i+1] == '*' {
					return "", xerrors.New("** must occupy an entire path segment")
				}
				_, _ = out.WriteString("[^/]*")
				continue
			}
			if ch == '?' {
				_, _ = out.WriteString("[^/]")
				continue
			}
			writeMemoryRegexLiteral(&out, ch)
		}
	}
	_ = out.WriteByte('$')
	return out.String(), nil
}

func compileMemoryDirectoryGlob(pattern string) (string, error) {
	base, err := compileMemoryGlob(pattern)
	if err != nil {
		return "", err
	}
	if pattern == "/" {
		return "^/", nil
	}
	return strings.TrimSuffix(base, "$") + "(/.*)?$", nil
}

func writeMemoryRegexLiteral(out *strings.Builder, ch byte) {
	if strings.ContainsRune(`.*+?()[]{}^$|\\`, rune(ch)) {
		_ = out.WriteByte('\\')
	}
	_ = out.WriteByte(ch)
}

type memoryLine struct {
	text string
	sep  string
}

type memoryDocument struct {
	lines     []memoryLine
	preferred string
}

func parseMemoryDocument(content string) memoryDocument {
	doc := memoryDocument{preferred: "\n"}
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] != '\n' {
			continue
		}
		textEnd := i
		sep := "\n"
		if i > start && content[i-1] == '\r' {
			textEnd--
			sep = "\r\n"
		}
		if len(doc.lines) == 0 {
			doc.preferred = sep
		}
		doc.lines = append(doc.lines, memoryLine{text: content[start:textEnd], sep: sep})
		start = i + 1
	}
	if start < len(content) {
		doc.lines = append(doc.lines, memoryLine{text: content[start:]})
	}
	if len(doc.lines) == 0 {
		doc.lines = []memoryLine{{}}
	}
	return doc
}

func memoryLineHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)[:3]
}

func renderMemoryHashlines(content string) string {
	doc := parseMemoryDocument(content)
	return renderMemoryHashlineRange(doc.lines, 0, len(doc.lines)-1)
}

func renderMemoryHashlineRange(lines []memoryLine, start, end int) string {
	var out strings.Builder
	for i := start; i <= end; i++ {
		_, _ = fmt.Fprintf(&out, "%d:%s|%s", i+1, memoryLineHash(lines[i].text), lines[i].text)
		_, _ = out.WriteString(lines[i].sep)
	}
	return out.String()
}

func parseMemoryAnchor(anchor string, lines []memoryLine) (int, error) {
	lineText, hash, ok := strings.Cut(anchor, ":")
	if !ok || len(hash) != 3 {
		return 0, xerrors.Errorf("invalid anchor %q; expected LINE_NUMBER:HASH", anchor)
	}
	for _, ch := range hash {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			return 0, xerrors.Errorf("invalid anchor %q; hash must be three lowercase hexadecimal characters", anchor)
		}
	}
	lineNumber, err := strconv.Atoi(lineText)
	if err != nil || lineNumber < 1 {
		return 0, xerrors.Errorf("invalid anchor %q; line number must be positive", anchor)
	}
	index := lineNumber - 1
	if index >= len(lines) {
		return 0, xerrors.Errorf("stale anchor %q; line no longer exists", anchor)
	}
	current := fmt.Sprintf("%d:%s", lineNumber, memoryLineHash(lines[index].text))
	if current != anchor {
		return 0, xerrors.Errorf("stale anchor %q; current anchor is %q", anchor, current)
	}
	return index, nil
}

type memorySpan struct {
	start       int
	end         int
	replacement []string
}

type memoryInsertion struct {
	anchor   int
	boundary int
	lines    []string
}

type memoryToken struct {
	text   string
	origin int
}

func applyMemoryEdits(content string, edits []memoryEdit) (string, error) {
	doc := parseMemoryDocument(content)
	spans := make([]memorySpan, 0, len(edits))
	insertions := make([]memoryInsertion, 0, len(edits))

	for _, edit := range edits {
		if err := validateMemoryEdit(edit); err != nil {
			return "", err
		}
		switch edit.Op {
		case "set_line":
			index, err := parseMemoryAnchor(edit.Anchor, doc.lines)
			if err != nil {
				return "", err
			}
			spans = append(spans, memorySpan{start: index, end: index, replacement: memoryBlockLines(edit.NewText)})
		case "replace_range":
			start, end, err := parseMemoryRange(edit, doc.lines)
			if err != nil {
				return "", err
			}
			spans = append(spans, memorySpan{start: start, end: end, replacement: memoryBlockLines(edit.NewText)})
		case "delete_line":
			index, err := parseMemoryAnchor(edit.Anchor, doc.lines)
			if err != nil {
				return "", err
			}
			spans = append(spans, memorySpan{start: index, end: index})
		case "delete_range":
			start, end, err := parseMemoryRange(edit, doc.lines)
			if err != nil {
				return "", err
			}
			spans = append(spans, memorySpan{start: start, end: end})
		case "insert_before", "insert_after":
			index, err := parseMemoryAnchor(edit.Anchor, doc.lines)
			if err != nil {
				return "", err
			}
			boundary := index
			if edit.Op == "insert_after" {
				boundary++
			}
			insertions = append(insertions, memoryInsertion{
				anchor:   index,
				boundary: boundary,
				lines:    memoryBlockLines(edit.NewText),
			})
		default:
			return "", xerrors.Errorf("unsupported memory edit operation %q", edit.Op)
		}
	}

	for i := range spans {
		for j := i + 1; j < len(spans); j++ {
			if spans[i].start <= spans[j].end && spans[j].start <= spans[i].end {
				return "", xerrors.New("memory edit ranges overlap")
			}
		}
		for _, insertion := range insertions {
			if insertion.anchor >= spans[i].start && insertion.anchor <= spans[i].end {
				return "", xerrors.New("insertion anchor is inside a replaced or deleted range")
			}
		}
	}

	spanByStart := make(map[int]memorySpan, len(spans))
	for _, span := range spans {
		spanByStart[span.start] = span
	}
	insertionsByBoundary := make(map[int][][]string, len(insertions))
	for _, insertion := range insertions {
		insertionsByBoundary[insertion.boundary] = append(
			insertionsByBoundary[insertion.boundary],
			insertion.lines,
		)
	}
	tokens := make([]memoryToken, 0, len(doc.lines))
	for index := 0; index <= len(doc.lines); {
		for _, block := range insertionsByBoundary[index] {
			tokens = appendMemoryTokens(tokens, block)
		}
		if index == len(doc.lines) {
			break
		}
		if span, ok := spanByStart[index]; ok {
			tokens = appendMemoryTokens(tokens, span.replacement)
			index = span.end + 1
			continue
		}
		tokens = append(tokens, memoryToken{text: doc.lines[index].text, origin: index})
		index++
	}

	return serializeMemoryTokens(doc, tokens), nil
}

func validateMemoryEdit(edit memoryEdit) error {
	singleAnchor := edit.Op == "set_line" || edit.Op == "insert_before" ||
		edit.Op == "insert_after" || edit.Op == "delete_line"
	rangeAnchors := edit.Op == "replace_range" || edit.Op == "delete_range"
	if !singleAnchor && !rangeAnchors {
		return xerrors.Errorf("unsupported memory edit operation %q", edit.Op)
	}
	if singleAnchor {
		if edit.Anchor == "" {
			return xerrors.Errorf("%s requires anchor", edit.Op)
		}
		if edit.StartAnchor != "" || edit.EndAnchor != "" {
			return xerrors.Errorf("%s does not accept range anchors", edit.Op)
		}
	}
	if rangeAnchors {
		if edit.StartAnchor == "" || edit.EndAnchor == "" {
			return xerrors.Errorf("%s requires start_anchor and end_anchor", edit.Op)
		}
		if edit.Anchor != "" {
			return xerrors.Errorf("%s does not accept anchor", edit.Op)
		}
	}
	if (edit.Op == "delete_line" || edit.Op == "delete_range") && edit.NewText != "" {
		return xerrors.Errorf("%s does not accept new_text", edit.Op)
	}
	return nil
}

func parseMemoryRange(edit memoryEdit, lines []memoryLine) (start, end int, err error) {
	start, err = parseMemoryAnchor(edit.StartAnchor, lines)
	if err != nil {
		return 0, 0, err
	}
	end, err = parseMemoryAnchor(edit.EndAnchor, lines)
	if err != nil {
		return 0, 0, err
	}
	if start > end {
		return 0, 0, xerrors.New("range start must not be after range end")
	}
	return start, end, nil
}

func memoryBlockLines(text string) []string {
	doc := parseMemoryDocument(text)
	lines := make([]string, len(doc.lines))
	for i, line := range doc.lines {
		lines[i] = line.text
	}
	return lines
}

func appendMemoryTokens(tokens []memoryToken, lines []string) []memoryToken {
	for _, line := range lines {
		tokens = append(tokens, memoryToken{text: line, origin: -1})
	}
	return tokens
}

func serializeMemoryTokens(original memoryDocument, tokens []memoryToken) string {
	if len(tokens) == 0 {
		return ""
	}
	finalNewline := original.lines[len(original.lines)-1].sep != ""
	var out strings.Builder
	for i, token := range tokens {
		_, _ = out.WriteString(token.text)
		if i+1 < len(tokens) {
			if token.origin >= 0 && original.lines[token.origin].sep != "" {
				_, _ = out.WriteString(original.lines[token.origin].sep)
			} else {
				_, _ = out.WriteString(original.preferred)
			}
			continue
		}
		if token.origin >= 0 {
			_, _ = out.WriteString(original.lines[token.origin].sep)
		} else if finalNewline {
			_, _ = out.WriteString(original.preferred)
		}
	}
	return out.String()
}

func memoryExcerpts(content, headline string) []memoryExcerpt {
	doc := parseMemoryDocument(content)
	headlineDoc := parseMemoryDocument(headline)
	hits := make([]int, 0, 3)
	for i, line := range headlineDoc.lines {
		if strings.Contains(line.text, memoryHeadlineStartMarker) && i < len(doc.lines) {
			hits = append(hits, i)
			if len(hits) == 3 {
				break
			}
		}
	}
	if len(hits) == 0 {
		return []memoryExcerpt{{
			StartLine: 1,
			EndLine:   1,
			Content:   renderMemoryHashlineRange(doc.lines, 0, 0),
		}}
	}

	type excerptRange struct{ start, end int }
	ranges := make([]excerptRange, 0, len(hits))
	for _, hit := range hits {
		start := max(0, hit-1)
		end := min(len(doc.lines)-1, hit+1)
		if len(ranges) > 0 && start <= ranges[len(ranges)-1].end+1 {
			ranges[len(ranges)-1].end = max(ranges[len(ranges)-1].end, end)
			continue
		}
		ranges = append(ranges, excerptRange{start: start, end: end})
	}

	remaining := 15
	excerpts := make([]memoryExcerpt, 0, len(ranges))
	for _, excerptRange := range ranges {
		if remaining == 0 {
			break
		}
		end := min(excerptRange.end, excerptRange.start+remaining-1)
		excerpts = append(excerpts, memoryExcerpt{
			StartLine: excerptRange.start + 1,
			EndLine:   end + 1,
			Content:   renderMemoryHashlineRange(doc.lines, excerptRange.start, end),
		})
		remaining -= end - excerptRange.start + 1
	}
	return excerpts
}
