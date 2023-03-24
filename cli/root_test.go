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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

// To update the golden files:
// make update-golden-files
var updateGoldenFiles = flag.Bool("update", false, "update .golden files")

var timestampRegex = regexp.MustCompile(`(?i)\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(.\d+)?Z`)

func TestCommandHelp(t *testing.T) {
	t.Parallel()
	rootClient, replacements := prepareTestData(t)

	type testCase struct {
		name string
		cmd  []string
	}
	tests := []testCase{
		{
			name: "coder --help",
			cmd:  []string{"--help"},
		},
		{
			name: "coder server --help",
			cmd:  []string{"server", "--help"},
		},
		{
			name: "coder agent --help",
			cmd:  []string{"agent", "--help"},
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
	root, err := rootCmd.Command(rootCmd.AGPL())
	require.NoError(t, err)

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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			var outBuf bytes.Buffer
			inv, cfg := clitest.New(t, tt.cmd...)
			inv.Stderr = &outBuf
			inv.Stdout = &outBuf
			inv.Environ.Set("CODER_URL", rootClient.URL.String())
			inv.Environ.Set("CODER_SESSION_TOKEN", rootClient.SessionToken())
			inv.Environ.Set("CODER_CACHE_DIRECTORY", "~/.cache")

			clitest.SetupConfig(t, rootClient, cfg)

			clitest.StartWithWaiter(t, inv.WithContext(ctx)).RequireSuccess()

			actual := outBuf.Bytes()
			if len(actual) == 0 {
				t.Fatal("no output")
			}

			for k, v := range replacements {
				actual = bytes.ReplaceAll(actual, []byte(k), []byte(v))
			}

			// Replace any timestamps with a placeholder.
			actual = timestampRegex.ReplaceAll(actual, []byte("[timestamp]"))

			homeDir, err := os.UserHomeDir()
			require.NoError(t, err)

			configDir := config.DefaultDir()
			actual = bytes.ReplaceAll(actual, []byte(configDir), []byte("~/.config/coderv2"))

			actual = bytes.ReplaceAll(actual, []byte(codersdk.DefaultCacheDir()), []byte("[cache dir]"))

			// The home directory changes depending on the test environment.
			actual = bytes.ReplaceAll(actual, []byte(homeDir), []byte("~"))

			goldenPath := filepath.Join("testdata", strings.Replace(tt.name, " ", "_", -1)+".golden")
			if *updateGoldenFiles {
				t.Logf("update golden file for: %q: %s", tt.name, goldenPath)
				err = os.WriteFile(goldenPath, actual, 0o600)
				require.NoError(t, err, "update golden file")
			}

			expected, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "read golden file, run \"make update-golden-files\" and commit the changes")

			// Normalize files to tolerate different operating systems.
			for _, r := range []struct {
				old string
				new string
			}{
				{"\r\n", "\n"},
				{`~\.cache\coder`, "~/.cache/coder"},
				{`C:\Users\RUNNER~1\AppData\Local\Temp`, "/tmp"},
				{os.TempDir(), "/tmp"},
			} {
				expected = bytes.ReplaceAll(expected, []byte(r.old), []byte(r.new))
				actual = bytes.ReplaceAll(actual, []byte(r.old), []byte(r.new))
			}
			require.Equal(
				t, string(expected), string(actual),
				"golden file mismatch: %s, run \"make update-golden-files\", verify and commit the changes",
				goldenPath,
			)
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
