package cli_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/acarl005/stripansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

// To update the golden files:
// make update-golden-files
var updateGoldenFiles = flag.Bool("update", false, "update .golden files")

var timestampRegex = regexp.MustCompile(`(?i)\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(.\d+)?Z`)

//nolint:tparallel,paralleltest // These test sets env vars.
func TestCommandHelp(t *testing.T) {
	commonEnv := map[string]string{
		"HOME":             "~",
		"CODER_CONFIG_DIR": "~/.config/coderv2",
	}

	rootClient, replacements := prepareTestData(t)

	type testCase struct {
		name string
		cmd  []string
		env  map[string]string
	}
	tests := []testCase{
		{
			name: "coder --help",
			cmd:  []string{"--help"},
		},
		{
			name: "coder server --help",
			cmd:  []string{"server", "--help"},
			env: map[string]string{
				"CODER_CACHE_DIRECTORY": "~/.cache/coder",
			},
		},
		{
			name: "coder agent --help",
			cmd:  []string{"agent", "--help"},
			env: map[string]string{
				"CODER_AGENT_LOG_DIR": "/tmp",
			},
		},
		{
			name: "coder list --output json",
			cmd:  []string{"list", "--output", "json"},
		},
		{
			name: "coder users list --output json",
			cmd:  []string{"users", "list", "--output", "json"},
		},
	}

	rootCmd := new(cli.RootCmd)
	root := rootCmd.Command(rootCmd.AGPL())
ExtractCommandPathsLoop:
	for _, cp := range extractVisibleCommandPaths(nil, root.Children) {
		name := fmt.Sprintf("coder %s --help", strings.Join(cp, " "))
		cmd := append(cp, "--help")
		for _, tt := range tests {
			if tt.name == name {
				continue ExtractCommandPathsLoop
			}
		}
		tests = append(tests, testCase{name: name, cmd: cmd})
	}

	wd, err := os.Getwd()
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		wd = strings.ReplaceAll(wd, "\\", "\\\\")
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			env := make(map[string]string)
			for k, v := range commonEnv {
				env[k] = v
			}
			for k, v := range tt.env {
				env[k] = v
			}

			// Unset all CODER_ environment variables for a clean slate.
			for _, kv := range os.Environ() {
				name := strings.Split(kv, "=")[0]
				if _, ok := env[name]; !ok && strings.HasPrefix(name, "CODER_") {
					t.Setenv(name, "")
				}
			}
			// Override environment variables for a reproducible test.
			for k, v := range env {
				t.Setenv(k, v)
			}

			ctx, _ := testutil.Context(t)

			tmpwd := "/"
			if runtime.GOOS == "windows" {
				tmpwd = "C:\\"
			}
			err := os.Chdir(tmpwd)
			var buf bytes.Buffer
			inv, cfg := clitest.New(t, tt.cmd...)
			clitest.SetupConfig(t, rootClient, cfg)
			inv.Stderr = &buf
			assert.NoError(t, err)
			err = inv.WithContext(ctx).Run()
			err2 := os.Chdir(wd)
			require.NoError(t, err)
			require.NoError(t, err2)

			got := buf.Bytes()

			replace := map[string][]byte{
				// Remove CRLF newlines (Windows).
				string([]byte{'\r', '\n'}): []byte("\n"),
				// The `coder templates create --help` command prints the path
				// to the working directory (--directory flag default value).
				fmt.Sprintf("%q", tmpwd): []byte("\"[current directory]\""),
			}
			for k, v := range replacements {
				replace[k] = []byte(v)
			}
			for k, v := range replace {
				got = bytes.ReplaceAll(got, []byte(k), v)
			}

			got = []byte(stripansi.Strip(string(got)))

			// Replace any timestamps with a placeholder.
			got = timestampRegex.ReplaceAll(got, []byte("[timestamp]"))

			gf := filepath.Join("testdata", strings.Replace(tt.name, " ", "_", -1)+".golden")
			if *updateGoldenFiles {
				t.Logf("update golden file for: %q: %s", tt.name, gf)
				err = os.WriteFile(gf, got, 0o600)
				require.NoError(t, err, "update golden file")
			}

			want, err := os.ReadFile(gf)
			require.NoError(t, err, "read golden file, run \"make update-golden-files\" and commit the changes")
			// Remove CRLF newlines (Windows).
			want = bytes.ReplaceAll(want, []byte{'\r', '\n'}, []byte{'\n'})
			require.Equal(t, string(want), string(got), "golden file mismatch: %s, run \"make update-golden-files\", verify and commit the changes", gf)
		})
	}
}

func extractVisibleCommandPaths(cmdPath []string, cmds []*clibase.Cmd) [][]string {
	var cmdPaths [][]string
	for _, c := range cmds {
		if c.Hidden {
			continue
		}
		cmdPath := append(cmdPath, c.Name())
		cmdPaths = append(cmdPaths, cmdPath)
		cmdPaths = append(cmdPaths, extractVisibleCommandPaths(cmdPath, c.Children)...)
	}
	return cmdPaths
}

func prepareTestData(t *testing.T) (*codersdk.Client, map[string]string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	db, pubsub := dbtestutil.NewDB(t)
	rootClient := coderdtest.New(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   pubsub,
		IncludeProvisionerDaemon: true,
	})
	firstUser := coderdtest.CreateFirstUser(t, rootClient)
	secondUser, err := rootClient.CreateUser(ctx, codersdk.CreateUserRequest{
		Email:          "testuser2@coder.com",
		Username:       "testuser2",
		Password:       coderdtest.FirstUserParams.Password,
		OrganizationID: firstUser.OrganizationID,
	})
	require.NoError(t, err)
	version := coderdtest.CreateTemplateVersion(t, rootClient, firstUser.OrganizationID, nil)
	version = coderdtest.AwaitTemplateVersionJob(t, rootClient, version.ID)
	template := coderdtest.CreateTemplate(t, rootClient, firstUser.OrganizationID, version.ID, func(req *codersdk.CreateTemplateRequest) {
		req.Name = "test-template"
	})
	workspace := coderdtest.CreateWorkspace(t, rootClient, firstUser.OrganizationID, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
		req.Name = "test-workspace"
	})
	workspaceBuild := coderdtest.AwaitWorkspaceBuildJob(t, rootClient, workspace.LatestBuild.ID)

	replacements := map[string]string{
		firstUser.UserID.String():            "[first user ID]",
		secondUser.ID.String():               "[second user ID]",
		firstUser.OrganizationID.String():    "[first org ID]",
		version.ID.String():                  "[version ID]",
		version.Name:                         "[version name]",
		version.Job.ID.String():              "[version job ID]",
		version.Job.FileID.String():          "[version file ID]",
		version.Job.WorkerID.String():        "[version worker ID]",
		template.ID.String():                 "[template ID]",
		workspace.ID.String():                "[workspace ID]",
		workspaceBuild.ID.String():           "[workspace build ID]",
		workspaceBuild.Job.ID.String():       "[workspace build job ID]",
		workspaceBuild.Job.FileID.String():   "[workspace build file ID]",
		workspaceBuild.Job.WorkerID.String(): "[workspace build worker ID]",
	}

	return rootClient, replacements
}

func TestRoot(t *testing.T) {
	t.Parallel()
	t.Run("Version", func(t *testing.T) {
		t.Parallel()

		buf := new(bytes.Buffer)
		inv, _ := clitest.New(t, "version")
		inv.Stdout = buf
		err := inv.Run()
		require.NoError(t, err)

		output := buf.String()
		require.Contains(t, output, buildinfo.Version(), "has version")
		require.Contains(t, output, buildinfo.ExternalURL(), "has url")
	})

	t.Run("Header", func(t *testing.T) {
		t.Parallel()

		done := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "wow", r.Header.Get("X-Testing"))
			w.WriteHeader(http.StatusGone)
			select {
			case <-done:
				close(done)
			default:
			}
		}))
		defer srv.Close()
		buf := new(bytes.Buffer)
		inv, _ := clitest.New(t, "--header", "X-Testing=wow", "login", srv.URL)
		inv.Stdout = buf
		// This won't succeed, because we're using the login cmd to assert requests.
		_ = inv.Run()
	})
}
