package chattool

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/agentbox"
)

// Box tool names.
const (
	BoxRunToolName        = "box_run"
	BoxWriteFileToolName  = "box_write_file"
	BoxReadFileToolName   = "box_read_file"
	BoxAttachFileToolName = "box_attach_file"
)

// BoxToolNames lists every box tool.
func BoxToolNames() []string {
	return []string{BoxRunToolName, BoxWriteFileToolName, BoxReadFileToolName, BoxAttachFileToolName}
}

// maxBoxOutputToModel caps each captured stream in a box_run result.
// BoxOptions.ResultBudgetBytes can lower it further.
const maxBoxOutputToModel = maxOutputToModel

// GetBoxFunc returns the current turn's box, creating it on first use.
// reset reports that the box replaced one the turn already used, so files
// from earlier calls are gone; it is true at most once per step.
type GetBoxFunc func(ctx context.Context) (box *agentbox.Box, reset bool, err error)

// BoxOptions configures the box tools.
type BoxOptions struct {
	GetBox GetBoxFunc
	// Languages lists the runtimes the engine accepts, for the box_run
	// description.
	Languages []string
	// Limits are the engine's effective limits, for tool descriptions.
	Limits agentbox.Limits
	// StoreFile persists box_attach_file output as a chat attachment.
	StoreFile StoreFileFunc
	// AttachFile reports that box_attach_file is usable in the turn, so
	// the box_run description may point to it.
	AttachFile bool
	// ResultBudgetBytes caps the marshaled box_run result. A result cut
	// past this size is no longer valid JSON, so both streams shrink until
	// the result fits. Zero applies only the per-stream cap.
	ResultBudgetBytes int
}

// boxTool runs box calls in tool-call order within a step so a script sees
// files written by an earlier call in the same step.
type boxTool struct {
	fantasy.AgentTool
}

func (boxTool) SerialToolCalls() bool { return true }

// BoxRunArgs are the arguments for box_run.
type BoxRunArgs struct {
	Language string   `json:"language" description:"Runtime to use. See the tool description for the accepted values."`
	Code     string   `json:"code" description:"Complete program source. Files under /box persist between calls in the current turn."`
	Stdin    string   `json:"stdin,omitempty" description:"Text supplied on standard input."`
	Args     []string `json:"args,omitempty" description:"Command-line arguments visible to the program."`
}

// BoxRun returns the box_run tool.
func BoxRun(options BoxOptions) fantasy.AgentTool {
	description := "Run a program in a temporary sandbox that has no workspace, network, or package access. " +
		"Languages: " + strings.Join(options.Languages, ", ") + ". " +
		"JavaScript runs on QuickJS with the std and os modules available as globals. " +
		"The program itself runs from /script, so import modules staged in the box by absolute path, for example import { f } from \"/box/lib.mjs\". " +
		"The program can read and write files under /box, which is private to the current turn and deleted when the turn ends. " +
		"Limits: " + options.Limits.RunTimeout.String() + " per run, " +
		byteCountString(int64(options.Limits.MemoryBytes)) + " memory, " +
		byteCountString(options.Limits.DiskBytes) + " under /box. " +
		"stdout and stderr are returned; output past " + byteCountString(maxBoxOutputToModel) + " per stream, or past the tool result size limit, is dropped and flagged. " +
		"exit_code is -1 when timed_out or canceled is true. " +
		"Use box_write_file to stage inputs and box_read_file to inspect outputs."
	if options.AttachFile {
		description += " Use box_attach_file to hand a result file to the user."
	}
	return boxTool{fantasy.NewAgentTool(
		BoxRunToolName,
		description,
		func(ctx context.Context, args BoxRunArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(args.Language) == "" {
				return fantasy.NewTextErrorResponse("language is required"), nil
			}
			if strings.TrimSpace(args.Code) == "" {
				return fantasy.NewTextErrorResponse("code is required"), nil
			}
			h, err := options.getBox(ctx)
			if err != nil {
				return boxErrorResponse(err), nil
			}
			result, err := h.box.Run(ctx, agentbox.RunRequest{
				Language: args.Language,
				Code:     args.Code,
				Stdin:    args.Stdin,
				Args:     args.Args,
			})
			if err != nil {
				return h.errorResponse(err), nil
			}
			stdout, stdoutCut := truncateBoxOutput(result.Stdout)
			stderr, stderrCut := truncateBoxOutput(result.Stderr)
			fields := h.withIdentity(map[string]any{
				"exit_code":   result.ExitCode,
				"timed_out":   result.TimedOut,
				"canceled":    result.Canceled,
				"duration_ms": result.Duration.Milliseconds(),
			})
			stdout, stderr, fitCut := fitStreams(fields, stdout, stderr, options.ResultBudgetBytes)
			fields["stdout"] = stdout
			fields["stderr"] = stderr
			fields["stdout_truncated"] = result.StdoutTruncated || stdoutCut || fitCut[0]
			fields["stderr_truncated"] = result.StderrTruncated || stderrCut || fitCut[1]
			return toolResponse(fields), nil
		},
	)}
}

// BoxWriteFileArgs are the arguments for box_write_file.
type BoxWriteFileArgs struct {
	Path    string `json:"path" description:"Path under /box, for example /box/input.csv or input.csv. Parent directories are created."`
	Content string `json:"content" description:"Complete file contents. Replaces any existing contents."`
}

// BoxWriteFile returns the box_write_file tool.
func BoxWriteFile(options BoxOptions) fantasy.AgentTool {
	return boxTool{fantasy.NewAgentTool(
		BoxWriteFileToolName,
		"Create or overwrite a file under /box in the current turn's sandbox so a later box_run can read it. "+
			"Files are deleted when the turn ends.",
		func(ctx context.Context, args BoxWriteFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(args.Path) == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}
			h, err := options.getBox(ctx)
			if err != nil {
				return boxErrorResponse(err), nil
			}
			if err := h.box.WriteFile(args.Path, []byte(args.Content)); err != nil {
				return h.errorResponse(err), nil
			}
			return h.response(map[string]any{
				"path":  args.Path,
				"bytes": len(args.Content),
			}), nil
		},
	)}
}

// BoxReadFileArgs are the arguments for box_read_file.
type BoxReadFileArgs struct {
	Path   string `json:"path" description:"Path under /box."`
	Offset *int   `json:"offset,omitempty" description:"1-based line number to start from (default 1)."`
	Limit  *int   `json:"limit,omitempty" description:"Number of lines to return (default 2000)."`
}

// BoxReadFile returns the box_read_file tool.
func BoxReadFile(options BoxOptions) fantasy.AgentTool {
	return boxTool{fantasy.NewAgentTool(
		BoxReadFileToolName,
		"Read a file under /box from the current turn's sandbox. Returns line-numbered content. "+
			"Use offset and limit to paginate large files.",
		func(ctx context.Context, args BoxReadFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(args.Path) == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}
			h, err := options.getBox(ctx)
			if err != nil {
				return boxErrorResponse(err), nil
			}
			offset, limit := 1, 0
			if args.Offset != nil {
				offset = *args.Offset
			}
			if args.Limit != nil {
				limit = *args.Limit
			}
			result, err := h.box.ReadLines(args.Path, offset, limit)
			if err != nil {
				return h.errorResponse(err), nil
			}
			return h.response(map[string]any{
				"content":     result.Content,
				"file_size":   result.FileSize,
				"total_lines": result.TotalLines,
				"lines_read":  result.LinesRead,
			}), nil
		},
	)}
}

// BoxAttachFileArgs are the arguments for box_attach_file.
type BoxAttachFileArgs struct {
	Path string `json:"path" description:"Path under /box of the file to attach."`
	Name string `json:"name,omitempty" description:"Attachment name shown to the user. Defaults to the file name."`
}

// BoxAttachFile returns the box_attach_file tool.
func BoxAttachFile(options BoxOptions) fantasy.AgentTool {
	return boxTool{fantasy.NewAgentTool(
		BoxAttachFileToolName,
		"Attach a file from /box in the current turn's sandbox to the chat so the user can download it. "+
			"Use this to deliver a result before the turn ends and the sandbox is deleted.",
		func(ctx context.Context, args BoxAttachFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			path := strings.TrimSpace(args.Path)
			if path == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}
			if options.StoreFile == nil {
				return fantasy.NewTextErrorResponse("file storage is not configured"), nil
			}
			h, err := options.getBox(ctx)
			if err != nil {
				return boxErrorResponse(err), nil
			}
			data, err := h.box.ReadFile(path, maxAttachmentSize+1)
			if err != nil {
				return h.errorResponse(err), nil
			}
			name := strings.TrimSpace(args.Name)
			if name == "" {
				trimmed := strings.TrimRight(path, "/")
				if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
					name = trimmed[idx+1:]
				} else {
					name = trimmed
				}
			}
			attachment, err := storeAttachmentData(ctx, options.StoreFile, name, path, data)
			if err != nil {
				return h.errorResponse(err), nil
			}
			return WithAttachments(h.response(map[string]any{
				"ok":         true,
				"path":       path,
				"file_id":    attachment.FileID.String(),
				"name":       attachment.Name,
				"media_type": attachment.MediaType,
				"size":       len(data),
			}), attachment), nil
		},
	)}
}

// boxHandle is the box a call operates on and whether it replaced one the
// turn already used.
type boxHandle struct {
	box   *agentbox.Box
	reset bool
}

func (o BoxOptions) getBox(ctx context.Context) (boxHandle, error) {
	if o.GetBox == nil {
		return boxHandle{}, xerrors.New("agent boxes are not configured")
	}
	box, reset, err := o.GetBox(ctx)
	if err != nil {
		return boxHandle{}, err
	}
	return boxHandle{box: box, reset: reset}, nil
}

// withIdentity adds the box identity to a result. box_id lets a later
// step detect that the box changed; box_reset tells the model that files
// from earlier in the turn are gone.
func (h boxHandle) withIdentity(result map[string]any) map[string]any {
	result["box_id"] = h.box.ID()
	if h.reset {
		result["box_reset"] = true
	}
	return result
}

func (h boxHandle) response(result map[string]any) fantasy.ToolResponse {
	return toolResponse(h.withIdentity(result))
}

// errorResponse reports a failed operation on an obtained box. It carries
// the box identity so a reset is not lost behind the error.
func (h boxHandle) errorResponse(err error) fantasy.ToolResponse {
	return toolResponse(h.withIdentity(boxErrorResult(err)))
}

// boxErrorResponse reports a failure to obtain a box.
func boxErrorResponse(err error) fantasy.ToolResponse {
	return toolResponse(boxErrorResult(err))
}

// boxErrorResult maps sandbox errors to a structured result. Capacity
// errors are flagged so the model retries instead of giving up.
func boxErrorResult(err error) map[string]any {
	result := map[string]any{"error": err.Error()}
	switch {
	case errors.Is(err, agentbox.ErrBusy), errors.Is(err, agentbox.ErrTooManyBoxes):
		result["retryable"] = true
		result["hint"] = "the sandbox is at capacity on this server; retry shortly"
	case errors.Is(err, agentbox.ErrUnknownLanguage):
		result["hint"] = "use one of the languages listed in the box_run description"
	}
	return result
}

func truncateBoxOutput(output string) (string, bool) {
	if len(output) <= maxBoxOutputToModel {
		return output, false
	}
	return strings.ToValidUTF8(output[:maxBoxOutputToModel], ""), true
}

// fitStreams shrinks stdout and stderr so fields marshaled with both
// streams and their truncation flags stay within budget bytes. cut
// reports which streams it shortened. A budget of zero or less leaves the
// streams unchanged.
func fitStreams(fields map[string]any, stdout, stderr string, budget int) (fitOut, fitErr string, cut [2]bool) {
	if budget <= 0 {
		return stdout, stderr, cut
	}
	outCost, errCost := jsonStringCost(stdout), jsonStringCost(stderr)
	probe := maps.Clone(fields)
	probe["stdout"], probe["stderr"] = stdout, stderr
	probe["stdout_truncated"], probe["stderr_truncated"] = false, false
	if whole, err := json.Marshal(probe); err == nil && len(whole) <= budget {
		return stdout, stderr, cut
	}
	probe["stdout"], probe["stderr"] = "", ""
	overhead, err := json.Marshal(probe)
	if err != nil {
		return "", "", [2]bool{stdout != "", stderr != ""}
	}
	avail := max(0, budget-len(overhead))
	outBudget, errBudget := avail/2, avail-avail/2
	switch {
	case errCost <= errBudget:
		outBudget, errBudget = avail-errCost, errCost
	case outCost <= outBudget:
		outBudget, errBudget = outCost, avail-outCost
	}
	if outCost > outBudget {
		stdout, cut[0] = jsonStringPrefix(stdout, outBudget), true
	}
	if errCost > errBudget {
		stderr, cut[1] = jsonStringPrefix(stderr, errBudget), true
	}
	return stdout, stderr, cut
}

// jsonRuneCost is an upper bound on the bytes encoding/json writes for a
// rune inside a string, including HTML escaping.
func jsonRuneCost(r rune, size int) int {
	switch {
	case r == '"', r == '\\', r == '\n', r == '\r', r == '\t':
		return 2
	case r == utf8.RuneError && size == 1, r < 0x20, r == '<', r == '>', r == '&', r == '\u2028', r == '\u2029':
		return 6
	default:
		return size
	}
}

// jsonStringCost bounds the encoded size of s without its quotes.
func jsonStringCost(s string) int {
	cost := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		cost += jsonRuneCost(r, size)
		i += size
	}
	return cost
}

// jsonStringPrefix returns the longest rune-aligned prefix of s whose
// encoded size is at most budget.
func jsonStringPrefix(s string, budget int) string {
	cost := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		cost += jsonRuneCost(r, size)
		if cost > budget {
			return s[:i]
		}
		i += size
	}
	return s
}

func byteCountString(n int64) string {
	switch {
	case n >= 1<<30 && n%(1<<30) == 0:
		return strconv.FormatInt(n>>30, 10) + " GiB"
	case n >= 1<<20 && n%(1<<20) == 0:
		return strconv.FormatInt(n>>20, 10) + " MiB"
	case n >= 1<<10 && n%(1<<10) == 0:
		return strconv.FormatInt(n>>10, 10) + " KiB"
	default:
		return strconv.FormatInt(n, 10) + " bytes"
	}
}
