//go:build !windows

package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/x/agentnative"
	"github.com/coder/serpent"
)

func invokeNative(t *testing.T, home string, environment []string, args ...string) (stdoutText, stderrText string, err error) {
	t.Helper()
	return invokeNativeWithAuthFile(t, home, "/run/secrets/claude-auth.json", environment, args...)
}

func invokeNativeWithAuthFile(t *testing.T, home, authFile string, environment []string, args ...string) (stdoutText, stderrText string, err error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	inv := nativeAgentWithAuthFile(authFile).Invoke(args...).WithContext(t.Context())
	inv.Environ = serpent.ParseEnviron(append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, environment...), "")
	inv.Stdin, inv.Stdout, inv.Stderr = strings.NewReader(""), &stdout, &stderr
	err = inv.Run()
	return stdout.String(), stderr.String(), err
}

func TestNativeAgentConfigure(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	project := filepath.Join(home, "project")
	require.NoError(t, os.Mkdir(project, 0o700))
	prompt := "Read $(touch /not-a-command); `literal` $HOME ${CODER_AGENT_TOKEN}\n'quotes' 🚀"
	env := []string{"CODER_NATIVE_JOB_PROMPT_BASE64=" + base64.StdEncoding.EncodeToString([]byte(prompt))}
	stdout, stderr, err := invokeNative(t, home, env, "configure")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	dir := agentnative.DefaultStateDir(home)
	data, err := nativeReadPrivate(filepath.Join(dir, "config.json"))
	require.NoError(t, err)
	var config agentnative.Config
	require.NoError(t, json.Unmarshal(data, &config))
	require.Equal(t, &agentnative.JobSpec{Harness: "claude", Cwd: project, Prompt: prompt}, config.InitialJob)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.EqualValues(t, 0o700, info.Mode().Perm())
	info, err = os.Stat(filepath.Join(dir, "config.json"))
	require.NoError(t, err)
	require.EqualValues(t, 0o600, info.Mode().Perm())
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1, "configuration must not create a manager, native runtime, or dispatch record")

	// Existing configuration short-circuits setup, even without auth or trust.
	_, _, err = invokeNative(t, home, env, "configure", "--setup")
	require.NoError(t, err)
	_, _, err = invokeNative(t, home, env, "configure", "--harness", "codex")
	require.ErrorContains(t, err, "differs")
	require.NotContains(t, err.Error(), prompt)
	after, err := os.ReadFile(filepath.Join(dir, "config.json"))
	require.NoError(t, err)
	require.Equal(t, data, after)
}

func TestNativeAgentConfigureRejectsLegacyJob(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"completed", "unknown", "malformed", "directory", "dangling-symlink", "legacy-directory-symlink"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			legacy := filepath.Join(home, ".local", "state", "coder-acp")
			require.NoError(t, os.MkdirAll(legacy, 0o700))
			jobFile := filepath.Join(legacy, "job.json")
			contents := []byte(`{"status":"` + kind + `","private":"retained-native-history"}`)
			switch kind {
			case "directory":
				require.NoError(t, os.Mkdir(jobFile, 0o700))
			case "dangling-symlink":
				require.NoError(t, os.Symlink(filepath.Join(home, "missing"), jobFile))
			case "legacy-directory-symlink":
				require.NoError(t, os.Remove(legacy))
				require.NoError(t, os.Symlink(filepath.Join(home, "missing"), legacy))
			default:
				if kind == "malformed" {
					contents = []byte("retained-native-history: invalid JSON")
				}
				require.NoError(t, os.WriteFile(jobFile, contents, 0o600))
			}
			for range 2 {
				stdout, stderr, err := invokeNative(t, home, []string{"CODER_NATIVE_JOB_PROMPT_BASE64=dGFzaw=="}, "configure", "--setup")
				require.ErrorContains(t, err, "legacy initial-job state")
				require.NotContains(t, err.Error(), "retained-native-history")
				require.Empty(t, stdout)
				require.Empty(t, stderr)
				_, err = os.Stat(filepath.Join(agentnative.DefaultStateDir(home), "config.json"))
				require.ErrorIs(t, err, os.ErrNotExist)
				_, err = os.Stat(filepath.Join(home, "project"))
				require.ErrorIs(t, err, os.ErrNotExist, "no setup or native operation may run")
			}
			if kind == "completed" || kind == "unknown" || kind == "malformed" {
				after, err := os.ReadFile(jobFile)
				require.NoError(t, err)
				require.Equal(t, contents, after)
			}
		})
	}
}

func TestNativeAgentClaudeHelperMigration(t *testing.T) {
	t.Parallel()
	for _, helper := range []string{"node /opt/native-agent/read-auth.mjs", "coder native-agent read-auth", "custom-auth-helper", "node /opt/native-agent/read-auth.mjs --custom"} {
		t.Run(helper, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			root := filepath.Join(home, ".claude")
			require.NoError(t, os.Mkdir(root, 0o700))
			settingsFile := filepath.Join(root, "settings.json")
			encoded, err := json.Marshal(helper)
			require.NoError(t, err)
			original := []byte(`{"apiKeyHelper":` + string(encoded) + `,"model":"user-model","largeNumber":9007199254740993,"env":{"ANTHROPIC_BASE_URL":"https://user.example.com"},"permissions":{"allow":["Read"]}}`)
			require.NoError(t, os.WriteFile(settingsFile, original, 0o600))
			configFile := filepath.Join(home, ".claude.json")
			credentialsFile := filepath.Join(root, ".credentials.json")
			for _, path := range []string{configFile, credentialsFile} {
				require.NoError(t, os.WriteFile(path, []byte(`{"retained":"private"}`), 0o600))
			}
			require.NoError(t, nativeMigrateClaudeHelper(home))
			after, err := nativeReadPrivate(settingsFile)
			require.NoError(t, err)
			if helper == "node /opt/native-agent/read-auth.mjs" {
				var expected, actual map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(original, &expected))
				require.NoError(t, json.Unmarshal(after, &actual))
				expected["apiKeyHelper"] = json.RawMessage(`"coder native-agent read-auth"`)
				require.Equal(t, expected, actual)
				require.Contains(t, string(after), "9007199254740993")
			} else {
				require.Equal(t, original, after)
			}
			require.NoError(t, nativeMigrateClaudeHelper(home))
			again, err := os.ReadFile(settingsFile)
			require.NoError(t, err)
			require.Equal(t, after, again)
			for _, path := range []string{configFile, credentialsFile} {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, `{"retained":"private"}`, string(data))
			}
		})
	}
}

func TestNativeAgentClaudeHelperMigrationBeforeLegacyRefusal(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	root := filepath.Join(home, ".claude")
	require.NoError(t, os.Mkdir(root, 0o700))
	settingsFile := filepath.Join(root, "settings.json")
	require.NoError(t, os.WriteFile(settingsFile, []byte(`{"apiKeyHelper":"node /opt/native-agent/read-auth.mjs"}`), 0o600))
	legacy := filepath.Join(home, ".local", "state", "coder-acp")
	require.NoError(t, os.MkdirAll(legacy, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "job.json"), []byte(`{"status":"unknown"}`), 0o600))
	_, _, err := invokeNative(t, home, []string{"CODER_NATIVE_JOB_PROMPT_BASE64=dGFzaw=="}, "configure", "--setup")
	require.ErrorContains(t, err, "legacy initial-job state")
	data, err := nativeReadPrivate(settingsFile)
	require.NoError(t, err)
	require.JSONEq(t, `{"apiKeyHelper":"coder native-agent read-auth"}`, string(data))
}

func TestNativeAgentClaudeHelperPreservesConcurrentEdit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.json")
	original := []byte(`{"apiKeyHelper":"node /opt/native-agent/read-auth.mjs"}`)
	changed := []byte(`{"apiKeyHelper":"user-updated-helper"}`)
	require.NoError(t, os.WriteFile(path, changed, 0o600))
	require.ErrorContains(t, nativeReplacePrivate(path, original, []byte(`{}`)), "changed")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, changed, after)
}

func TestNativeAgentConfigureValidation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, prompt, harness, cwd string }{
		{name: "encoding", prompt: "not-base64"},
		{name: "empty", prompt: base64.StdEncoding.EncodeToString([]byte(" \n"))},
		{name: "invalid-utf8", prompt: "/w=="},
		{name: "too-long", prompt: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32001)))},
		{name: "harness", prompt: "dGFzaw==", harness: "other"},
		{name: "relative-directory", prompt: "dGFzaw==", cwd: "relative"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			args := []string{"configure"}
			if test.harness != "" {
				args = append(args, "--harness", test.harness)
			}
			if test.cwd != "" {
				args = append(args, "--cwd", test.cwd)
			}
			stdout, stderr, err := invokeNative(t, home, []string{"CODER_NATIVE_JOB_PROMPT_BASE64=" + test.prompt}, args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			_, err = os.Stat(filepath.Join(agentnative.DefaultStateDir(home), "config.json"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestNativeAgentConfigureConcurrent(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(home, "project"), 0o700))
	var wait sync.WaitGroup
	for range 8 {
		wait.Go(func() {
			_, _, err := invokeNative(t, home, []string{"CODER_NATIVE_JOB_PROMPT_BASE64=dGFzaw=="}, "configure")
			require.NoError(t, err)
		})
	}
	wait.Wait()
	data, err := nativeReadPrivate(filepath.Join(agentnative.DefaultStateDir(home), "config.json"))
	require.NoError(t, err)
	require.True(t, json.Valid(data))
}

func writeNativeDescriptor(t *testing.T, home, url, token string) string {
	t.Helper()
	dir := agentnative.DefaultStateDir(home)
	require.NoError(t, nativePrivateDirectory(dir))
	data, err := json.Marshal(nativeDescriptor{URL: url, Token: token})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manager.json"), data, 0o600))
	return dir
}

func TestNativeAgentDescriptor(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{
		"https://127.0.0.1:1234", "http://localhost:1234", "http://example.com:1234",
		"http://192.168.1.1:1234", "http://0.0.0.0:1234", "http://127.0.0.1", "http://127.0.0.1:0",
		"http://127.0.0.1:65536", "http://token@127.0.0.1:1234", "http://127.0.0.1:1234/path",
		"http://127.0.0.1:1234?token=secret", "http://127.0.0.1:1234#secret", "http://127.0.0.1:1234?",
	} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := writeNativeDescriptor(t, home, endpoint, "private-test-token")
			_, err := nativeReadDescriptor(dir)
			require.Error(t, err)
			require.NotContains(t, err.Error(), endpoint)
			require.NotContains(t, err.Error(), "private-test-token")
		})
	}
	for _, endpoint := range []string{"http://127.0.0.1:1234", "http://[::1]:1234/"} {
		dir := writeNativeDescriptor(t, t.TempDir(), endpoint, "private-test-token")
		_, err := nativeReadDescriptor(dir)
		require.NoError(t, err)
	}
}

func TestNativeAgentPrivateFiles(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"public-file", "symlink-file", "public-dir", "symlink-dir", "malformed", "invalid-token"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := writeNativeDescriptor(t, home, "http://127.0.0.1:1234", "private-test-token")
			file := filepath.Join(dir, "manager.json")
			switch kind {
			case "public-file":
				require.NoError(t, os.Chmod(file, 0o644))
			case "public-dir":
				require.NoError(t, os.Chmod(dir, 0o755))
			case "symlink-file":
				require.NoError(t, os.Rename(file, file+".target"))
				require.NoError(t, os.Symlink(file+".target", file))
			case "symlink-dir":
				require.NoError(t, os.Rename(dir, dir+"-target"))
				require.NoError(t, os.Symlink(dir+"-target", dir))
			case "malformed":
				require.NoError(t, os.WriteFile(file, []byte("private-test-token"), 0o600))
			case "invalid-token":
				writeNativeDescriptor(t, home, "http://127.0.0.1:1234", "secret\nheader")
			}
			stdout, stderr, err := invokeNative(t, home, nil, "shutdown")
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.NotContains(t, err.Error(), "private-test-token")
		})
	}
}

func TestNativeAgentAttach(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	id := "opencode:ses_CasePreservingID"
	secret := uuid.NewString()
	// A local fake terminal proves literal argv, environment, cwd and stdio.
	terminal := filepath.Join(home, "terminal")
	require.NoError(t, os.WriteFile(terminal, []byte("#!/bin/sh\n[ \"$NATIVE_CREDENTIAL\" = '"+secret+"' ] || exit 2\n[ \"$PWD\" = \"$EXPECTED_DIR\" ] || exit 3\nprintf '%s' \"$1\"\n"), 0o600))
	require.NoError(t, os.Chmod(terminal, 0o700))
	arg := "$(not-a-command); $NATIVE_CREDENTIAL `literal`"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/sessions/"+id+"/attach", r.URL.Path)
		require.Equal(t, "Bearer private-test-token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewEncoder(w).Encode(agentnative.Command{Path: filepath.Base(terminal), Args: []string{arg}, Dir: home, Env: []string{"PATH=" + home, "NATIVE_CREDENTIAL=" + secret, "EXPECTED_DIR=" + home}}))
	}))
	defer server.Close()
	writeNativeDescriptor(t, home, server.URL, "private-test-token")
	stdout, stderr, err := invokeNative(t, home, nil, "attach", id)
	require.NoError(t, err)
	require.Equal(t, arg, stdout)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, secret)
}

func TestNativeAgentAttachInvalidID(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"../shutdown", "id/attach", "$(touch /tmp/oops)", "opencode:ses_has/slash"} {
		stdout, stderr, err := invokeNative(t, t.TempDir(), nil, "attach", id)
		require.ErrorContains(t, err, "invalid native session ID")
		require.Empty(t, stdout)
		require.Empty(t, stderr)
	}
}

func TestNativeAgentHTTPFailures(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError, http.StatusTemporaryRedirect, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			secret := "private-response-must-not-leak"
			destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("must not follow redirects") }))
			defer destination.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer private-test-token", r.Header.Get("Authorization"))
				w.Header().Set("Location", destination.URL)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(secret))
			}))
			defer server.Close()
			writeNativeDescriptor(t, home, server.URL, "private-test-token")
			stdout, stderr, err := invokeNative(t, home, nil, "attach", uuid.NewString())
			require.Error(t, err)
			require.NotContains(t, err.Error(), secret)
			require.NotContains(t, err.Error(), "private-test-token")
			require.Empty(t, stdout)
			require.Empty(t, stderr)
		})
	}
}

func TestNativeAgentShutdown(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/shutdown", r.URL.Path)
		require.Equal(t, "Bearer private-test-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("private shutdown output"))
	}))
	defer server.Close()
	dir := writeNativeDescriptor(t, home, server.URL, "private-test-token")
	stdout, stderr, err := invokeNative(t, home, nil, "shutdown")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	_, err = os.Stat(filepath.Join(dir, "manager.json"))
	require.NoError(t, err, "shutdown must not delete local runtime state")
}

func TestNativeAgentReadAuth(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "auth.json")
	secret := "private-key"
	require.NoError(t, os.WriteFile(file, []byte(`{"ANTHROPIC_API_KEY":"`+secret+`"}`), 0o600))
	var output bytes.Buffer
	require.NoError(t, nativeReadAuth(file, &output))
	require.Equal(t, secret+"\n", output.String())
	for _, data := range []string{secret, `{"ANTHROPIC_API_KEY":"secret\nkey"}`, `{"ANTHROPIC_AUTH_TOKEN":"` + secret + `"}`} {
		require.NoError(t, os.WriteFile(file, []byte(data), 0o600))
		output.Reset()
		err := nativeReadAuth(file, &output)
		require.Error(t, err)
		require.Empty(t, output.String())
		require.NotContains(t, err.Error(), secret)
	}
}

func TestNativeAgentConfigurePromptFile(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	project := filepath.Join(home, "project")
	require.NoError(t, os.Mkdir(project, 0o700))
	promptFile := filepath.Join(home, "prompt")
	prompt := "literal $HOME\n$(not-a-command)"
	require.NoError(t, os.WriteFile(promptFile, []byte(prompt), 0o600))
	_, _, err := invokeNative(t, home, nil, "configure", "--prompt-file", promptFile)
	require.NoError(t, err)
	config, err := agentnative.ReadConfig(agentnative.DefaultStateDir(home))
	require.NoError(t, err)
	require.Equal(t, prompt, config.InitialJob.Prompt)
	// A malformed retained config is not silently repaired.
	configFile := filepath.Join(agentnative.DefaultStateDir(home), "config.json")
	require.NoError(t, os.WriteFile(configFile, []byte("private invalid JSON"), 0o600))
	_, _, err = invokeNative(t, home, nil, "configure", "--prompt-file", promptFile)
	require.ErrorContains(t, err, "differs or is invalid")
	require.NotContains(t, err.Error(), "private invalid JSON")
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	require.Equal(t, "private invalid JSON", string(data))
}

func TestNativeAgentCodexSetup(t *testing.T) {
	t.Parallel()
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			bin := filepath.Join(home, "bin")
			codexHome := filepath.Join(home, ".codex")
			require.NoError(t, os.Mkdir(bin, 0o700))
			require.NoError(t, os.Mkdir(codexHome, 0o700))
			if existing {
				require.NoError(t, os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"retained":"auth"}`), 0o600))
			}
			script := `#!/bin/sh
set -eu
umask 077
[ -z "${CODER_NATIVE_JOB_PROMPT_BASE64+x}" ]
[ -z "${CODER_NATIVE_JOB_CODEX_API_KEY_BASE64+x}" ]
[ -z "${CODER_AGENT_TOKEN+x}" ]
[ "$1" = login ]
if [ -f "$CODEX_HOME/auth.json" ]; then
  [ "$2" = status ]
else
  [ "$2" = --with-api-key ]
  read -r key
  [ "$key" = 'private-test-key' ]
  printf '{"key":"%s"}' "$key" > "$CODEX_HOME/auth.json"
fi
printf private-test-key
printf private-test-key >&2
`
			require.NoError(t, os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o600))
			require.NoError(t, os.Chmod(filepath.Join(bin, "codex"), 0o700))
			mount := filepath.Join(home, "auth-mount.json")
			require.NoError(t, os.WriteFile(mount, []byte(`{"ANTHROPIC_API_KEY":"private-claude-key","OPENAI_API_KEY":"private-test-key"}`), 0o600))
			if existing {
				// Native authentication takes precedence even over an unusable mount.
				require.NoError(t, os.WriteFile(mount, []byte("invalid mount"), 0o600))
			}
			stdout, stderr, err := invokeNativeWithAuthFile(t, home, mount, []string{
				"PATH=" + bin, "CODEX_HOME=" + codexHome, "CODER_AGENT_TOKEN=private-agent-key",
				"CODER_NATIVE_JOB_HARNESS=codex", "CODER_NATIVE_JOB_TRUST=true", "CODER_NATIVE_JOB_PROMPT_BASE64=dGFzaw==",
				"CODER_NATIVE_JOB_CODEX_API_KEY_BASE64=" + base64.StdEncoding.EncodeToString([]byte("ignored-legacy-key")),
			}, "configure", "--setup")
			require.NoError(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			data, err := nativeReadPrivate(filepath.Join(codexHome, "auth.json"))
			require.NoError(t, err)
			if existing {
				require.JSONEq(t, `{"retained":"auth"}`, string(data))
			}
		})
	}
}

func TestNativeAgentCodexRejectsUnsafeMount(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"missing", "public", "symlink", "invalid-json", "claude-only", "whitespace", "nul"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			mount := filepath.Join(home, "mount.json")
			contents := `{"OPENAI_API_KEY":"private-mount-key"}`
			switch kind {
			case "invalid-json":
				contents = "private-mount-key"
			case "claude-only":
				contents = `{"ANTHROPIC_API_KEY":"private-mount-key"}`
			case "whitespace":
				contents = `{"OPENAI_API_KEY":"private-mount-key\n"}`
			case "nul":
				contents = `{"OPENAI_API_KEY":"private-mount-key\u0000"}`
			}
			if kind != "missing" {
				require.NoError(t, os.WriteFile(mount, []byte(contents), 0o600))
			}
			if kind == "public" {
				require.NoError(t, os.Chmod(mount, 0o644))
			}
			if kind == "symlink" {
				require.NoError(t, os.Rename(mount, mount+".target"))
				require.NoError(t, os.Symlink(mount+".target", mount))
			}
			stdout, stderr, err := invokeNativeWithAuthFile(t, home, mount, []string{
				"CODER_NATIVE_JOB_HARNESS=codex", "CODER_NATIVE_JOB_TRUST=true", "CODER_NATIVE_JOB_PROMPT_BASE64=dGFzaw==",
				"CODER_NATIVE_JOB_CODEX_API_KEY_BASE64=" + base64.StdEncoding.EncodeToString([]byte("ignored-legacy-key")),
			}, "configure", "--setup")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-mount-key")
			require.NotContains(t, err.Error(), "ignored-legacy-key")
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			_, err = os.Stat(filepath.Join(agentnative.DefaultStateDir(home), "config.json"))
			require.ErrorIs(t, err, os.ErrNotExist)
			_, err = os.Stat(filepath.Join(home, ".codex", "auth.json"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestNativeAgentClaudeSetup(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"api-key", "oauth", "gateway"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			project := filepath.Join(home, "project")
			mount := filepath.Join(home, "mount.json")
			secret := "private-authentication"
			auth := map[string]any{"ANTHROPIC_API_KEY": secret}
			if kind == "oauth" {
				auth = map[string]any{"claudeAiOauth": map[string]string{"accessToken": secret}}
			}
			if kind == "gateway" {
				auth = map[string]any{"ANTHROPIC_AUTH_TOKEN": secret, "ANTHROPIC_BASE_URL": "https://provider.example.com"}
			}
			data, err := json.Marshal(auth)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(mount, data, 0o600))
			require.NoError(t, nativeSetupClaude(home, project, "haiku", mount))
			file := filepath.Join(home, ".claude", "settings.json")
			original, err := nativeReadPrivate(file)
			require.NoError(t, err)
			if kind == "api-key" {
				require.Contains(t, string(original), "coder native-agent read-auth")
				require.NotContains(t, string(original), secret)
			}
			// Removing the mount or changing a model must not replace native auth.
			require.NoError(t, os.Remove(mount))
			require.NoError(t, nativeSetupClaude(home, project, "opus", mount))
			after, err := nativeReadPrivate(file)
			require.NoError(t, err)
			require.Equal(t, original, after)
		})
	}
}
