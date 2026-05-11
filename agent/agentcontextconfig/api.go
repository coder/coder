package agentcontextconfig

import (
	"cmp"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

// Env var names for context configuration. Prefixed with EXP_
// to indicate these are experimental and may change.
const (
	EnvInstructionsDirs = "CODER_AGENT_EXP_INSTRUCTIONS_DIRS"
	EnvInstructionsFile = "CODER_AGENT_EXP_INSTRUCTIONS_FILE"
	EnvSkillsDirs       = "CODER_AGENT_EXP_SKILLS_DIRS"
	EnvSkillMetaFile    = "CODER_AGENT_EXP_SKILL_META_FILE"
	EnvMCPConfigFiles   = "CODER_AGENT_EXP_MCP_CONFIG_FILES"
)

const (
	maxInstructionFileBytes = 64 * 1024
	maxSkillMetaBytes       = 64 * 1024
)

// markdownCommentPattern strips HTML comments from instruction
// file content for security (prevents hidden prompt injection).
var markdownCommentPattern = regexp.MustCompile(`<!--[\s\S]*?-->`)

// invisibleRunePattern strips invisible Unicode characters that
// could be used for prompt injection.
//
//nolint:gocritic // Non-ASCII char ranges are intentional for invisible Unicode stripping.
var invisibleRunePattern = regexp.MustCompile(
	"[\u00ad\u034f\u061c\u070f" +
		"\u115f\u1160\u17b4\u17b5" +
		"\u180b-\u180f" +
		"\u200b\u200d\u200e\u200f" +
		"\u202a-\u202e" +
		"\u2060-\u206f" +
		"\u3164" +
		"\ufe00-\ufe0f" +
		"\ufeff" +
		"\uffa0" +
		"\ufff0-\ufff8]",
)

// skillNamePattern validates kebab-case skill names.
var skillNamePattern = regexp.MustCompile(
	`^[a-z0-9]+(-[a-z0-9]+)*$`,
)

// Default values for agent-internal configuration. These are
// used when the corresponding env vars are unset.
const (
	DefaultInstructionsDir  = "~/.coder"
	DefaultInstructionsFile = "AGENTS.md"
	DefaultSkillsDir        = ".agents/skills"
	DefaultSkillMetaFile    = "SKILL.md"
	DefaultMCPConfigFile    = ".mcp.json"
)

// Config holds the agent's context configuration.
// Defaults are applied by NewAPI, not by the zero value.
type Config struct {
	InstructionsDirs string
	InstructionsFile string
	SkillsDirs       string
	SkillMetaFile    string
	MCPConfigFiles   string
}

// applyDefaults fills zero-valued fields with their defaults.
func (c Config) applyDefaults() Config {
	c.InstructionsDirs = cmp.Or(c.InstructionsDirs, DefaultInstructionsDir)
	c.InstructionsFile = cmp.Or(c.InstructionsFile, DefaultInstructionsFile)
	c.SkillsDirs = cmp.Or(c.SkillsDirs, DefaultSkillsDir)
	c.SkillMetaFile = cmp.Or(c.SkillMetaFile, DefaultSkillMetaFile)
	c.MCPConfigFiles = cmp.Or(c.MCPConfigFiles, DefaultMCPConfigFile)
	return c
}

// ReadEnvConfig reads the CODER_AGENT_EXP_* environment
// variables, falling back to defaults for unset values.
func ReadEnvConfig() Config {
	return Config{
		InstructionsDirs: strings.TrimSpace(os.Getenv(EnvInstructionsDirs)),
		InstructionsFile: strings.TrimSpace(os.Getenv(EnvInstructionsFile)),
		SkillsDirs:       strings.TrimSpace(os.Getenv(EnvSkillsDirs)),
		SkillMetaFile:    strings.TrimSpace(os.Getenv(EnvSkillMetaFile)),
		MCPConfigFiles:   strings.TrimSpace(os.Getenv(EnvMCPConfigFiles)),
	}.applyDefaults()
}

// envVarKeys returns every CODER_AGENT_EXP_* env var key
// used by the context configuration subsystem.
func envVarKeys() []string {
	return []string{
		EnvInstructionsDirs, EnvInstructionsFile,
		EnvSkillsDirs, EnvSkillMetaFile, EnvMCPConfigFiles,
	}
}

// ClearEnvVars removes the CODER_AGENT_EXP_* environment
// variables from the current process so they are not
// inherited by child processes.
func ClearEnvVars() {
	for _, key := range envVarKeys() {
		_ = os.Unsetenv(key)
	}
}

// API exposes the resolved context configuration through the
// agent's HTTP API.
type API struct {
	workingDir func() string
	cfg        Config
	logger     slog.Logger

	// clock is injectable so tests can drive the startup-gate
	// timer without sleeping.
	clock quartz.Clock

	// startupSettled is closed once startup scripts reach a
	// terminal state. Before that, handleGet may return a result
	// based on a still-empty filesystem because startup scripts
	// have not yet written AGENTS.md or .agents/skills/.
	//
	// The shape mirrors agent/x/agentmcp.Manager. If a third
	// caller appears, factor the gate into a shared helper.
	startupSettled chan struct{}
	startupOnce    sync.Once

	// startupTimeout caps how long handleGet waits before
	// falling back to reading whatever is on disk. Better a real
	// answer than an indefinite hang.
	startupTimeout time.Duration
}

// Option configures a new API.
type Option func(*API)

// WithClock overrides the API's clock. Tests pass a mock to
// drive startupTimeout deterministically.
func WithClock(c quartz.Clock) Option {
	return func(a *API) { a.clock = c }
}

// NewAPI creates a context configuration API. The working
// directory closure is evaluated lazily per request.
func NewAPI(logger slog.Logger, workingDir func() string, cfg Config, opts ...Option) *API {
	if workingDir == nil {
		workingDir = func() string { return "" }
	}
	a := &API{
		workingDir:     workingDir,
		cfg:            cfg.applyDefaults(),
		logger:         logger,
		clock:          quartz.NewReal(),
		startupSettled: make(chan struct{}),
		startupTimeout: 35 * time.Second,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// MarkStartupSettled marks startup scripts as terminal for
// context-config purposes. The first GET after this point
// proceeds without waiting. Concurrent and repeat calls are
// safe.
func (api *API) MarkStartupSettled() {
	api.startupOnce.Do(func() { close(api.startupSettled) })
}

// errStartupTimeout signals that the startup gate timed out
// before settle. handleGet treats this as a fallback path, not
// an error to the caller.
var errStartupTimeout = xerrors.New("startup gate timed out")

// waitForStartupSettled blocks until startup is marked settled,
// the request context is canceled, or startupTimeout elapses.
func (api *API) waitForStartupSettled(ctx context.Context) error {
	select {
	case <-api.startupSettled:
		return nil
	default:
	}
	timer := api.clock.NewTimer(api.startupTimeout, "agentcontextconfig", "startup_gate")
	defer timer.Stop()
	select {
	case <-api.startupSettled:
		return nil
	case <-timer.C:
		return errStartupTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Resolve reads instruction files, discovers skills, and
// resolves MCP config file paths for the given config and
// working directory.
func Resolve(workingDir string, cfg Config) (workspacesdk.ContextConfigResponse, []string) {
	resolvedInstructionsDirs := ResolvePaths(cfg.InstructionsDirs, workingDir)
	resolvedSkillsDirs := ResolvePaths(cfg.SkillsDirs, workingDir)

	// Read instruction files from each configured directory.
	parts := readInstructionFiles(resolvedInstructionsDirs, cfg.InstructionsFile)

	// Also check the working directory for the instruction file,
	// unless it was already covered by InstructionsDirs.
	if workingDir != "" {
		seenDirs := make(map[string]struct{}, len(resolvedInstructionsDirs))
		for _, d := range resolvedInstructionsDirs {
			seenDirs[d] = struct{}{}
		}
		if _, ok := seenDirs[workingDir]; !ok {
			if entry, found := readInstructionFileFromDir(workingDir, cfg.InstructionsFile); found {
				parts = append(parts, entry)
			}
		}
	}

	// Discover skills from each configured skills directory.
	skillParts := discoverSkills(resolvedSkillsDirs, cfg.SkillMetaFile)
	parts = append(parts, skillParts...)

	// Guarantee non-nil slice to signal agent support.
	if parts == nil {
		parts = []codersdk.ChatMessagePart{}
	}

	return workspacesdk.ContextConfigResponse{
		Parts: parts,
	}, ResolvePaths(cfg.MCPConfigFiles, workingDir)
}

// ContextPartsFromDir reads instruction files and discovers skills
// from a specific directory, using default file names. This is used
// by the CLI chat context commands to read context from an arbitrary
// directory without consulting agent env vars.
func ContextPartsFromDir(dir string) []codersdk.ChatMessagePart {
	var parts []codersdk.ChatMessagePart

	if entry, found := readInstructionFileFromDir(dir, DefaultInstructionsFile); found {
		parts = append(parts, entry)
	}

	// Reuse ResolvePaths so CLI skill discovery follows the same
	// project-relative path handling as agent config resolution.
	skillParts := discoverSkills(
		ResolvePaths(strings.Join([]string{DefaultSkillsDir, "skills"}, ","), dir),
		DefaultSkillMetaFile,
	)
	parts = append(parts, skillParts...)

	// Guarantee non-nil slice.
	if parts == nil {
		parts = []codersdk.ChatMessagePart{}
	}

	return parts
}

// MCPConfigFiles returns the resolved MCP configuration file
// paths for the agent's MCP manager.
func (api *API) MCPConfigFiles() []string {
	_, mcpFiles := Resolve(api.workingDir(), api.cfg)
	return mcpFiles
}

// Routes returns the HTTP handler for the context config
// endpoint.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", api.handleGet)
	return r
}

func (api *API) handleGet(rw http.ResponseWriter, r *http.Request) {
	if err := api.waitForStartupSettled(r.Context()); err != nil {
		if !errors.Is(err, errStartupTimeout) {
			httpapi.Write(r.Context(), rw, http.StatusServiceUnavailable, codersdk.Response{
				Message: "Agent startup has not settled.",
				Detail:  err.Error(),
			})
			return
		}
		// On timeout we fall back to whatever is on disk so a slow
		// or stuck startup script does not freeze chatd.
		api.logger.Warn(r.Context(),
			"context-config startup gate timed out, returning disk state",
			slog.F("timeout", api.startupTimeout),
		)
	}
	response, _ := Resolve(api.workingDir(), api.cfg)
	httpapi.Write(r.Context(), rw, http.StatusOK, response)
}

// readInstructionFiles reads instruction files from each given
// directory. Missing directories are silently skipped. Duplicate
// directories are deduplicated.
func readInstructionFiles(dirs []string, fileName string) []codersdk.ChatMessagePart {
	var parts []codersdk.ChatMessagePart
	seen := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		if part, found := readInstructionFileFromDir(dir, fileName); found {
			parts = append(parts, part)
		}
	}
	return parts
}

// readInstructionFileFromDir scans a directory for a file matching
// fileName (case-insensitive) and reads its contents.
func readInstructionFileFromDir(dir, fileName string) (codersdk.ChatMessagePart, bool) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return codersdk.ChatMessagePart{}, false
	}

	for _, e := range dirEntries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(e.Name()), fileName) {
			filePath := filepath.Join(dir, e.Name())
			content, truncated, ok := readAndSanitizeFile(filePath, maxInstructionFileBytes)
			if !ok {
				return codersdk.ChatMessagePart{}, false
			}
			if content == "" {
				return codersdk.ChatMessagePart{}, false
			}
			return codersdk.ChatMessagePart{
				Type:                 codersdk.ChatMessagePartTypeContextFile,
				ContextFilePath:      filePath,
				ContextFileContent:   content,
				ContextFileTruncated: truncated,
			}, true
		}
	}
	return codersdk.ChatMessagePart{}, false
}

// readAndSanitizeFile reads the file at path, capping the read
// at maxBytes to avoid unbounded memory allocation. It sanitizes
// the content (strips HTML comments and invisible Unicode) and
// returns the result. Returns false if the file cannot be read.
func readAndSanitizeFile(path string, maxBytes int64) (content string, truncated bool, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, false
	}
	defer f.Close()

	// Read at most maxBytes+1 to detect truncation without
	// allocating the entire file into memory.
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", false, false
	}

	truncated = int64(len(raw)) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}

	s := sanitizeInstructionMarkdown(string(raw))
	if s == "" {
		return "", truncated, true
	}
	return s, truncated, true
}

// sanitizeInstructionMarkdown strips HTML comments, invisible
// Unicode characters, and CRLF line endings from instruction
// file content.
func sanitizeInstructionMarkdown(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = markdownCommentPattern.ReplaceAllString(content, "")
	content = invisibleRunePattern.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

// discoverSkills walks the given skills directories and returns
// metadata for every valid skill it finds. Body and supporting
// file lists are NOT included; chatd fetches those on demand
// via read_skill. Missing directories or individual errors are
// silently skipped.
func discoverSkills(skillsDirs []string, metaFile string) []codersdk.ChatMessagePart {
	seen := make(map[string]struct{})
	var parts []codersdk.ChatMessagePart

	for _, skillsDir := range skillsDirs {
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			metaPath := filepath.Join(skillsDir, entry.Name(), metaFile)
			f, err := os.Open(metaPath)
			if err != nil {
				continue
			}
			raw, err := io.ReadAll(io.LimitReader(f, maxSkillMetaBytes+1))
			_ = f.Close()
			if err != nil {
				continue
			}
			if int64(len(raw)) > maxSkillMetaBytes {
				raw = raw[:maxSkillMetaBytes]
			}

			name, description, _, err := workspacesdk.ParseSkillFrontmatter(string(raw))
			if err != nil {
				continue
			}

			// The directory name must match the declared name.
			if name != entry.Name() {
				continue
			}
			if !skillNamePattern.MatchString(name) {
				continue
			}

			// First occurrence wins across directories.
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}

			skillDir := filepath.Join(skillsDir, entry.Name())
			parts = append(parts, codersdk.ChatMessagePart{
				Type:                     codersdk.ChatMessagePartTypeSkill,
				SkillName:                name,
				SkillDescription:         description,
				SkillDir:                 skillDir,
				ContextFileSkillMetaFile: metaFile,
			})
		}
	}

	return parts
}
