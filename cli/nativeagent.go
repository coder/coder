package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/x/agentnative"
	"github.com/coder/serpent"
)

const nativePrivateLimit = 1 << 20

var nativeSessionID = regexp.MustCompile(`^(?:(?:codex:)?[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}|opencode:ses_[A-Za-z0-9]+)$`)

func nativeAgent() *serpent.Command {
	return nativeAgentWithAuthFile("/run/secrets/claude-auth.json")
}

func nativeAgentWithAuthFile(authFile string) *serpent.Command {
	var stateDir, harness, cwd, promptFile string
	var setup bool
	state := func(inv *serpent.Invocation) (string, error) {
		dir := stateDir
		if dir == "" {
			home := inv.Environ.Get("HOME")
			if home == "" {
				var err error
				home, err = os.UserHomeDir()
				if err != nil {
					return "", xerrors.New("cannot determine native agent home")
				}
			}
			dir = agentnative.DefaultStateDir(home)
		}
		if !filepath.IsAbs(dir) {
			return "", xerrors.New("native agent state directory must be absolute")
		}
		return dir, nil
	}
	cmd := &serpent.Command{
		Use: "native-agent", Short: "Manage experimental native agents inside this workspace.", Hidden: true,
		Handler: func(inv *serpent.Invocation) error { return inv.Command.HelpHandler(inv) },
		Options: serpent.OptionSet{{Name: "State directory", Flag: "state-dir", Description: "Private native agent state directory.", Value: serpent.StringOf(&stateDir)}},
	}
	cmd.AddSubcommands(&serpent.Command{
		Use: "configure", Short: "Write initial-job configuration without contacting the manager.",
		Middleware: serpent.RequireNArgs(0),
		Options: serpent.OptionSet{
			{Name: "Harness", Flag: "harness", Description: "Native harness: claude, codex, or opencode.", Value: serpent.StringOf(&harness)},
			{Name: "Working directory", Flag: "cwd", Description: "Absolute project directory.", Value: serpent.StringOf(&cwd)},
			{Name: "Prompt file", Flag: "prompt-file", Description: "Private UTF-8 prompt file; otherwise use the encoded template environment.", Value: serpent.StringOf(&promptFile)},
			{Name: "Template setup", Flag: "setup", Description: "Prepare the template project and native authentication before saving configuration.", Value: serpent.BoolOf(&setup)},
		},
		Handler: func(inv *serpent.Invocation) error {
			dir, err := state(inv)
			if err != nil {
				return err
			}
			home := inv.Environ.Get("HOME")
			if home == "" {
				home, err = os.UserHomeDir()
			}
			if err != nil || !filepath.IsAbs(home) {
				return xerrors.New("configuration requires an absolute workspace home")
			}
			// Repair only our generated helper, even when a retained legacy job
			// prevents new initial-job configuration below.
			if setup {
				if err := nativeMigrateClaudeHelper(home); err != nil {
					return err
				}
			}
			if err := nativeRejectLegacyJob(home); err != nil {
				return err
			}
			selected := harness
			if selected == "" {
				selected = inv.Environ.Get("CODER_NATIVE_JOB_HARNESS")
			}
			if selected == "" {
				selected = "claude"
			}
			project := cwd
			if project == "" {
				project = filepath.Join(inv.Environ.Get("HOME"), "project")
			}
			var prompt string
			if promptFile != "" {
				data, err := nativeReadPrivate(promptFile)
				if err != nil {
					return xerrors.New("cannot read private prompt file")
				}
				prompt = string(data)
			} else {
				prompt, err = nativeDecodeParameter(inv.Environ.Get("CODER_NATIVE_JOB_PROMPT_BASE64"))
				if err != nil {
					return err
				}
			}
			config := agentnative.Config{InitialJob: &agentnative.JobSpec{Harness: selected, Cwd: project, Prompt: prompt}}
			if err := nativeValidateConfig(config); err != nil {
				return err
			}
			if err := nativePrivateDirectory(dir); err != nil {
				return err
			}
			path := filepath.Join(dir, "config.json")
			existing, err := nativeReadPrivate(path)
			configured := err == nil
			if configured {
				if err := nativeSameConfig(existing, config); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return xerrors.New("cannot read existing private native agent configuration")
			}
			if setup {
				if err := agentnative.PrepareDirectories(dir, append(inv.Environ.ToOS(), "HOME="+home)); err != nil {
					return xerrors.Errorf("prepare native directories: %w", err)
				}
			}
			if configured {
				return nil
			}
			if setup {
				if err := nativeTemplateSetup(inv, dir, config.InitialJob, authFile); err != nil {
					return err
				}
			}
			if info, err := os.Stat(project); err != nil || !info.IsDir() {
				return xerrors.New("native agent project directory does not exist")
			}
			data, err := json.Marshal(config)
			if err != nil {
				return xerrors.New("encode native agent configuration")
			}
			if err := nativeCreatePrivate(path, append(data, '\n')); err != nil {
				if errors.Is(err, os.ErrExist) {
					existing, readErr := nativeReadPrivate(path)
					if readErr == nil {
						return nativeSameConfig(existing, config)
					}
				}
				return xerrors.New("save private native agent configuration")
			}
			return nil
		},
	}, &serpent.Command{
		Use: "attach <session-id>", Short: "Attach a native terminal to an existing session.", Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			if len(inv.Args[0]) > 256 || !nativeSessionID.MatchString(inv.Args[0]) {
				return xerrors.New("invalid native session ID")
			}
			dir, err := state(inv)
			if err != nil {
				return err
			}
			var command agentnative.Command
			if err := nativeRequest(inv.Context(), dir, "/v1/sessions/"+url.PathEscape(inv.Args[0])+"/attach", &command); err != nil {
				return err
			}
			return nativeRunTerminal(inv, command)
		},
	}, &serpent.Command{
		Use: "shutdown", Short: "Explicitly stop native services, preserving saved conversations.", Middleware: serpent.RequireNArgs(0),
		Handler: func(inv *serpent.Invocation) error {
			dir, err := state(inv)
			if err != nil {
				return err
			}
			return nativeRequest(inv.Context(), dir, "/v1/shutdown", nil)
		},
	}, &serpent.Command{
		Use: "read-auth", Short: "Read the mounted Claude API key for apiKeyHelper only.", Middleware: serpent.RequireNArgs(0),
		Handler: func(inv *serpent.Invocation) error {
			return nativeReadAuth(authFile, inv.Stdout)
		},
	})
	return cmd
}

// A retained legacy job is evidence of an earlier dispatch even when its JSON
// is malformed or its outcome is unknown. Never infer that it is safe to retry.
func nativeRejectLegacyJob(home string) error {
	legacy := filepath.Join(home, ".local", "state", "coder-acp")
	info, err := os.Lstat(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil && info.IsDir() {
		_, err = os.Lstat(filepath.Join(legacy, "job.json"))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	return xerrors.New("legacy initial-job state exists or cannot be inspected in ~/.local/state/coder-acp; automatic initial-job configuration is disabled; inspect native history and submit new work explicitly, without deleting saved state")
}

const nativeClaudeAPIKeyHelper = "coder native-agent read-auth"

func nativeMigrateClaudeHelper(home string) error {
	root := filepath.Join(home, ".claude")
	path := filepath.Join(root, "settings.json")
	original, err := nativeReadPrivate(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return xerrors.New("cannot inspect private Claude settings for helper migration")
	}
	var settings map[string]json.RawMessage
	if json.Unmarshal(original, &settings) != nil || settings == nil {
		return xerrors.New("existing Claude settings are invalid; they were not replaced")
	}
	var helper string
	if json.Unmarshal(settings["apiKeyHelper"], &helper) != nil || helper != "node /opt/native-agent/read-auth.mjs" {
		return nil
	}
	if err := nativeCheckPrivateDirectory(root); err != nil {
		return err
	}
	settings["apiKeyHelper"], _ = json.Marshal(nativeClaudeAPIKeyHelper)
	updated, err := json.Marshal(settings)
	if err != nil {
		return xerrors.New("encode private Claude helper settings")
	}
	if err := nativeReplacePrivate(path, original, append(updated, '\n')); err != nil {
		return xerrors.New("Claude settings changed or helper migration could not be saved; inspect private settings")
	}
	return nil
}

// Recheck the complete private file before replacing it so an observed user
// edit is never overwritten by a migration prepared from an older snapshot.
func nativeReplacePrivate(path string, original, updated []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".native-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(updated); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	current, err := nativeReadPrivate(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return xerrors.New("private settings changed during migration")
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func nativeDecodeParameter(encoded string) (string, error) {
	if len(encoded) > 176000 {
		return "", xerrors.New("invalid encoded native job parameter")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(data) != encoded || !utf8.Valid(data) || bytes.ContainsRune(data, '\x00') {
		return "", xerrors.New("invalid encoded native job parameter")
	}
	return string(data), nil
}

func nativeValidateConfig(config agentnative.Config) error {
	job := config.InitialJob
	if job == nil || (job.Harness != "claude" && job.Harness != "codex" && job.Harness != "opencode") {
		return xerrors.New("choose the claude, codex, or opencode harness")
	}
	if !filepath.IsAbs(job.Cwd) || len(job.Cwd) > 4096 || strings.ContainsRune(job.Cwd, '\x00') {
		return xerrors.New("native agent project directory must be absolute")
	}
	if !utf8.ValidString(job.Prompt) || strings.ContainsRune(job.Prompt, '\x00') || strings.TrimSpace(job.Prompt) == "" || utf8.RuneCountInString(job.Prompt) > 32000 || len(job.Prompt) > 100000 {
		return xerrors.New("initial prompt must contain 1 to 32,000 UTF-8 characters, at most 100,000 bytes")
	}
	return nil
}

func nativeSameConfig(data []byte, want agentnative.Config) error {
	var existing agentnative.Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&existing) != nil || decoder.Decode(new(any)) != io.EOF || !reflect.DeepEqual(existing, want) {
		return xerrors.New("existing native agent configuration differs or is invalid; inspect it before changing the initial job")
	}
	return nil
}

type nativeDescriptor struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func nativeReadDescriptor(dir string) (nativeDescriptor, error) {
	var descriptor nativeDescriptor
	if err := nativeCheckPrivateDirectory(dir); err != nil {
		return descriptor, err
	}
	data, err := nativeReadPrivate(filepath.Join(dir, "manager.json"))
	if err != nil {
		return descriptor, xerrors.New("cannot read private native agent manager descriptor; wait for workspace startup")
	}
	if json.Unmarshal(data, &descriptor) != nil {
		return descriptor, xerrors.New("invalid native agent manager descriptor")
	}
	u, err := url.Parse(descriptor.URL)
	if err != nil {
		return descriptor, xerrors.New("invalid native agent manager address")
	}
	ip, ipErr := netip.ParseAddr(u.Hostname())
	port, portErr := strconv.ParseUint(u.Port(), 10, 16)
	if u.Scheme != "http" || ipErr != nil || !ip.IsLoopback() || ip.Zone() != "" || portErr != nil || port == 0 || u.User != nil || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return descriptor, xerrors.New("native agent manager address must be an HTTP loopback IP and port")
	}
	if len(descriptor.Token) < 16 || len(descriptor.Token) > 512 || strings.IndexFunc(descriptor.Token, func(r rune) bool { return r < 33 || r > 126 }) != -1 {
		return descriptor, xerrors.New("invalid native agent manager token")
	}
	descriptor.URL = strings.TrimRight(descriptor.URL, "/")
	return descriptor, nil
}

func nativeRequest(ctx context.Context, dir, path string, result any) error {
	descriptor, err := nativeReadDescriptor(dir)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, descriptor.URL+path, strings.NewReader("{}"))
	if err != nil {
		return xerrors.New("create native agent manager request")
	}
	request.Header.Set("Authorization", "Bearer "+descriptor.Token)
	request.Header.Set("Content-Type", "application/json")
	// No proxy or redirects: the private bearer must never leave loopback.
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 55 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return xerrors.New("native agent manager request failed; inspect local manager state")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Native diagnostics and response bodies can contain provider credentials.
		return xerrors.Errorf("native agent manager refused request (HTTP %d)", response.StatusCode)
	}
	if result != nil {
		data, err := io.ReadAll(io.LimitReader(response.Body, nativePrivateLimit+1))
		if err != nil || len(data) > nativePrivateLimit || json.Unmarshal(data, result) != nil {
			return xerrors.New("invalid native agent manager response")
		}
	}
	return nil
}

func nativeRunTerminal(inv *serpent.Invocation, command agentnative.Command) error {
	if command.Path == "" || strings.ContainsRune(command.Path, '\x00') || !filepath.IsAbs(command.Dir) || strings.ContainsRune(command.Dir, '\x00') {
		return xerrors.New("invalid native terminal command")
	}
	for _, arg := range command.Args {
		if strings.ContainsRune(arg, '\x00') {
			return xerrors.New("invalid native terminal argument")
		}
	}
	for _, env := range command.Env {
		name, _, ok := strings.Cut(env, "=")
		if !ok || name == "" || strings.ContainsRune(env, '\x00') {
			return xerrors.New("invalid native terminal environment")
		}
	}
	path, err := nativeExecutable(command.Path, command.Env)
	if err != nil {
		return err
	}
	// Execute argv directly, never a shell command assembled from native output.
	child := exec.CommandContext(inv.Context(), path, command.Args...)
	child.Dir, child.Env = command.Dir, append([]string{}, command.Env...)
	child.Stdin, child.Stdout, child.Stderr = inv.Stdin, inv.Stdout, inv.Stderr
	if err := child.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return &exitError{code: exit.ExitCode(), err: xerrors.New("native terminal exited")}
		}
		return xerrors.New("cannot start native terminal")
	}
	return nil
}

// Resolve bare executable names in the manager's environment, not an unrelated
// terminal shell's PATH. Relative PATH entries never select workspace files.
func nativeExecutable(path string, env []string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	if filepath.Base(path) == path {
		var searchPath string
		for _, entry := range env {
			if value, ok := strings.CutPrefix(entry, "PATH="); ok {
				searchPath = value
			}
		}
		for _, directory := range filepath.SplitList(searchPath) {
			if !filepath.IsAbs(directory) {
				continue
			}
			candidate := filepath.Join(directory, path)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
				return candidate, nil
			}
		}
	}
	return "", xerrors.New("native executable is unavailable in the configured PATH")
}

func nativeReadAuth(path string, output io.Writer) error {
	data, err := nativeReadPrivate(path)
	if err != nil {
		return xerrors.New("cannot read the private Claude API key")
	}
	var auth struct {
		Key string `json:"ANTHROPIC_API_KEY"`
	}
	if json.Unmarshal(data, &auth) != nil || auth.Key == "" || strings.IndexFunc(auth.Key, unicode.IsSpace) != -1 || strings.ContainsRune(auth.Key, '\x00') {
		return xerrors.New("invalid private Claude API key")
	}
	if _, err := io.WriteString(output, auth.Key+"\n"); err != nil {
		return xerrors.New("write Claude API key to helper output")
	}
	return nil
}

// nativeCreatePrivate publishes a fully written file without replacing any
// existing file, including when two configuration processes race.
func nativeCreatePrivate(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".native-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Link(file.Name(), path); err != nil {
		return err
	}
	if err := os.Remove(file.Name()); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func nativeTemplateSetup(inv *serpent.Invocation, stateDir string, job *agentnative.JobSpec, mount string) error {
	home := inv.Environ.Get("HOME")
	if !filepath.IsAbs(home) {
		return xerrors.New("template setup requires an absolute HOME")
	}
	if inv.Environ.Get("CODER_NATIVE_JOB_TRUST") != "true" {
		return xerrors.New("select Trust this project before unattended setup; native tool permissions are not bypassed")
	}
	repository, err := nativeDecodeParameter(inv.Environ.Get("CODER_NATIVE_JOB_REPO_BASE64"))
	if err != nil {
		return err
	}
	if repository != "" {
		u, err := url.Parse(repository)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return xerrors.New("repository must be a public HTTPS URL without embedded credentials")
		}
	}
	// Bootstrap secrets and the prompt are not inherited by git or native login.
	var env []string
	for _, value := range inv.Environ.ToOS() {
		if strings.HasPrefix(value, "CODER_NATIVE_JOB_") || strings.HasPrefix(value, "CODER_AGENT_") {
			continue
		}
		env = append(env, value)
	}
	run := func(path string, args []string, input string) error {
		path, err := nativeExecutable(path, env)
		if err != nil {
			return err
		}
		child := exec.CommandContext(inv.Context(), path, args...)
		child.Dir, child.Env, child.Stdin = home, env, strings.NewReader(input)
		// Do not forward provider output, even on failure.
		if err := child.Run(); err != nil {
			return xerrors.New("native template setup command failed; inspect native configuration")
		}
		return nil
	}
	if info, err := os.Lstat(job.Cwd); errors.Is(err, os.ErrNotExist) {
		if repository == "" {
			err = os.MkdirAll(job.Cwd, 0o700)
		} else {
			err = run("git", []string{"-c", "core.hooksPath=/dev/null", "-c", "protocol.file.allow=never", "clone", "--depth", "1", "--", repository, job.Cwd}, "")
		}
		if err != nil {
			return xerrors.New("prepare native project directory")
		}
	} else if err != nil || !info.IsDir() {
		return xerrors.New("native project must be an existing directory, not a symlink")
	} else if repository != "" {
		if _, err := os.Stat(filepath.Join(job.Cwd, ".git")); err != nil {
			return xerrors.New("project already exists without a Git checkout; inspect it before retrying")
		}
	}
	switch job.Harness {
	case "claude":
		return nativeSetupClaude(home, job.Cwd, inv.Environ.Get("CODER_NATIVE_JOB_MODEL"), mount)
	case "codex":
		codexHome := inv.Environ.Get("CODEX_HOME")
		if codexHome == "" {
			codexHome = filepath.Join(home, ".codex")
			env = append(env, "CODEX_HOME="+codexHome)
		}
		if !filepath.IsAbs(codexHome) {
			return xerrors.New("CODEX_HOME must be absolute")
		}
		if err := nativePrivateDirectory(codexHome); err != nil {
			return err
		}
		authFile := filepath.Join(codexHome, "auth.json")
		if _, err := nativeReadPrivate(authFile); err == nil {
			return run("codex", []string{"login", "status"}, "")
		} else if !errors.Is(err, os.ErrNotExist) {
			return xerrors.New("existing Codex authentication is not private; it was not replaced")
		}
		data, err := nativeReadPrivate(mount)
		if err != nil {
			return xerrors.New("provision native Codex authentication or OPENAI_API_KEY in the private mounted authentication file")
		}
		var auth struct {
			Key string `json:"OPENAI_API_KEY"`
		}
		if json.Unmarshal(data, &auth) != nil || auth.Key == "" || strings.IndexFunc(auth.Key, unicode.IsSpace) != -1 || strings.ContainsRune(auth.Key, '\x00') {
			return xerrors.New("invalid OpenAI API key in the private mounted authentication file")
		}
		if err := run("codex", []string{"login", "--with-api-key"}, auth.Key+"\n"); err != nil {
			return err
		}
		if _, err := nativeReadPrivate(authFile); err != nil {
			return xerrors.New("native Codex login did not create private authentication")
		}
	case "opencode":
		for _, path := range []string{"", "config", "data", "state", "cache", "tmp"} {
			if err := nativePrivateDirectory(filepath.Join(stateDir, "opencode-home", path)); err != nil {
				return err
			}
		}
	}
	return nil
}

func nativeSetupClaude(home, project, model, mount string) error {
	if err := nativeMigrateClaudeHelper(home); err != nil {
		return err
	}
	root := filepath.Join(home, ".claude")
	if err := nativePrivateDirectory(root); err != nil {
		return err
	}
	settingsFile := filepath.Join(root, "settings.json")
	credentialsFile := filepath.Join(root, ".credentials.json")
	settings := make(map[string]any)
	hasSettings, hasCredentials := false, false
	if data, err := nativeReadPrivate(settingsFile); err == nil {
		hasSettings = true
		if json.Unmarshal(data, &settings) != nil || settings == nil {
			return xerrors.New("existing Claude settings are invalid; they were not replaced")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return xerrors.New("existing Claude settings are not private; they were not replaced")
	}
	if data, err := nativeReadPrivate(credentialsFile); err == nil {
		hasCredentials = true
		if !json.Valid(data) {
			return xerrors.New("existing Claude credentials are invalid; they were not replaced")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return xerrors.New("existing Claude credentials are not private; they were not replaced")
	}
	if !hasSettings && !hasCredentials {
		data, err := nativeReadPrivate(mount)
		if err != nil {
			return xerrors.New("provision native Claude authentication or a private mounted Claude authentication file")
		}
		var auth struct {
			OAuth struct {
				AccessToken string `json:"accessToken"`
			} `json:"claudeAiOauth"`
			APIKey  string `json:"ANTHROPIC_API_KEY"`
			Token   string `json:"ANTHROPIC_AUTH_TOKEN"`
			BaseURL string `json:"ANTHROPIC_BASE_URL"`
		}
		if json.Unmarshal(data, &auth) != nil {
			return xerrors.New("invalid private Claude authentication")
		}
		switch {
		case auth.OAuth.AccessToken != "":
			if err := nativeCreatePrivate(credentialsFile, data); err != nil {
				return xerrors.New("save private Claude credentials")
			}
		case auth.APIKey != "":
			if strings.IndexFunc(auth.APIKey, unicode.IsSpace) != -1 {
				return xerrors.New("invalid private Claude API key")
			}
			settings["apiKeyHelper"] = nativeClaudeAPIKeyHelper
		case auth.Token != "" && auth.BaseURL != "":
			u, err := url.Parse(auth.BaseURL)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
				return xerrors.New("claude provider gateway must use HTTPS without embedded credentials")
			}
			settings["env"] = map[string]string{"ANTHROPIC_AUTH_TOKEN": auth.Token, "ANTHROPIC_BASE_URL": auth.BaseURL}
		default:
			return xerrors.New("claude authentication requires OAuth credentials, an API key, or an HTTPS gateway and token")
		}
	}
	if !hasSettings {
		if model == "" {
			model = "haiku"
		}
		if model != "haiku" && model != "sonnet" && model != "opus" {
			return xerrors.New("choose the haiku, sonnet, or opus Claude model")
		}
		settings["model"] = model
		data, _ := json.Marshal(settings)
		if err := nativeCreatePrivate(settingsFile, append(data, '\n')); err != nil {
			return xerrors.New("save private Claude settings")
		}
	}
	configFile := filepath.Join(home, ".claude.json")
	if data, err := nativeReadPrivate(configFile); err == nil {
		var config struct {
			Projects map[string]struct {
				Trusted bool `json:"hasTrustDialogAccepted"`
			} `json:"projects"`
		}
		if json.Unmarshal(data, &config) != nil || !config.Projects[project].Trusted {
			return xerrors.New("existing Claude configuration has not trusted this project; open Claude and confirm trust")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return xerrors.New("existing Claude configuration is not private; it was not replaced")
	}
	data, _ := json.Marshal(map[string]any{"hasCompletedOnboarding": true, "projects": map[string]any{project: map[string]bool{"hasTrustDialogAccepted": true}}})
	if err := nativeCreatePrivate(configFile, append(data, '\n')); err != nil {
		return xerrors.New("save private Claude project trust")
	}
	return nil
}
