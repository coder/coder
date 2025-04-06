package clitest

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// UpdateGoldenFiles indicates golden files should be updated.
// To update the golden files:
// make gen/golden-files
var UpdateGoldenFiles = flag.Bool("update", false, "update .golden files")

var timestampRegex = regexp.MustCompile(`(?i)\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(.\d+)?(Z|[+-]\d+:\d+)`)

type CommandHelpCase struct {
	Name string
	Cmd  []string
}

func DefaultCases() []CommandHelpCase {
	return []CommandHelpCase{
		{
			Name: "coder --help",
			Cmd:  []string{"--help"},
		},
		{
			Name: "coder server --help",
			Cmd:  []string{"server", "--help"},
		},
	}
}

// TestCommandHelp will test the help output of the given commands
// using golden files.
func TestCommandHelp(t *testing.T, getRoot func(t *testing.T) *serpent.Command, cases []CommandHelpCase) {
	t.Parallel()
	rootClient, replacements := prepareTestData(t)

	root := getRoot(t)

ExtractCommandPathsLoop:
	for _, cp := range extractVisibleCommandPaths(nil, root.Children) {
		name := fmt.Sprintf("coder %s --help", strings.Join(cp, " "))
		//nolint:gocritic
		cmd := append(cp, "--help")
		for _, tt := range cases {
			if tt.Name == name {
				continue ExtractCommandPathsLoop
			}
		}
		cases = append(cases, CommandHelpCase{Name: name, Cmd: cmd})
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			var outBuf bytes.Buffer

			caseCmd := getRoot(t)

			inv, cfg := NewWithCommand(t, caseCmd, tt.Cmd...)
			inv.Stderr = &outBuf
			inv.Stdout = &outBuf
			inv.Environ.Set("CODER_URL", rootClient.URL.String())
			inv.Environ.Set("CODER_SESSION_TOKEN", rootClient.SessionToken())
			inv.Environ.Set("CODER_CACHE_DIRECTORY", "~/.cache")

			SetupConfig(t, rootClient, cfg)

			StartWithWaiter(t, inv.WithContext(ctx)).RequireSuccess()

			TestGoldenFile(t, tt.Name, outBuf.Bytes(), replacements)
		})
	}
}

// TestGoldenFile will test the given bytes slice input against the
// golden file with the given file name, optionally using the given replacements.
func TestGoldenFile(t *testing.T, fileName string, actual []byte, replacements map[string]string) {
	if len(actual) == 0 {
		t.Fatal("no output")
	}

	for k, v := range replacements {
		actual = bytes.ReplaceAll(actual, []byte(k), []byte(v))
	}

	actual = normalizeGoldenFile(t, actual)
	goldenPath := filepath.Join("testdata", strings.ReplaceAll(fileName, " ", "_")+".golden")
	if *UpdateGoldenFiles {
		t.Logf("update golden file for: %q: %s", fileName, goldenPath)
		err := os.WriteFile(goldenPath, actual, 0o600)
		require.NoError(t, err, "update golden file")
	}

	expected, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "read golden file, run \"make gen/golden-files\" and commit the changes")

	expected = normalizeGoldenFile(t, expected)
	assert.Empty(t, cmp.Diff(string(expected), string(actual)), "golden file mismatch (-want +got): %s, run \"make gen/golden-files\", verify and commit the changes", goldenPath)
}

// normalizeGoldenFile replaces any strings that are system or timing dependent
// with a placeholder so that the golden files can be compared with a simple
// equality check.
func normalizeGoldenFile(t *testing.T, byt []byte) []byte {
	// Replace any timestamps with a placeholder.
	byt = timestampRegex.ReplaceAll(byt, []byte(pad("[timestamp]", 20)))

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	configDir := config.DefaultDir()
	byt = bytes.ReplaceAll(byt, []byte(configDir), []byte("~/.config/coderv2"))

	byt = bytes.ReplaceAll(byt, []byte(codersdk.DefaultCacheDir()), []byte("[cache dir]"))

	// The home directory changes depending on the test environment.
	byt = bytes.ReplaceAll(byt, []byte(homeDir), []byte("~"))
	for _, r := range []struct {
		old string
		new string
	}{
		{"\r\n", "\n"},
		{`~\.cache\coder`, "~/.cache/coder"},
		{`C:\Users\RUNNER~1\AppData\Local\Temp`, "/tmp"},
		{os.TempDir(), "/tmp"},
	} {
		byt = bytes.ReplaceAll(byt, []byte(r.old), []byte(r.new))
	}
	return byt
}

func extractVisibleCommandPaths(cmdPath []string, cmds []*serpent.Command) [][]string {
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

	// This needs to be a fixed timezone because timezones increase the length
	// of timestamp strings. The increased length can pad table formatting's
	// and differ the table header spacings.
	//nolint:gocritic
	db, pubsub := dbtestutil.NewDB(t, dbtestutil.WithTimezone("UTC"))
	rootClient := coderdtest.New(t, &coderdtest.Options{
		Database:                 db,
		Pubsub:                   pubsub,
		IncludeProvisionerDaemon: true,
	})
	firstUser := coderdtest.CreateFirstUser(t, rootClient)
	secondUser, err := rootClient.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
		Email:           "testuser2@coder.com",
		Username:        "testuser2",
		Password:        coderdtest.FirstUserParams.Password,
		OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
	})
	require.NoError(t, err)
	version := coderdtest.CreateTemplateVersion(t, rootClient, firstUser.OrganizationID, nil)
	version = coderdtest.AwaitTemplateVersionJobCompleted(t, rootClient, version.ID)
	template := coderdtest.CreateTemplate(t, rootClient, firstUser.OrganizationID, version.ID, func(req *codersdk.CreateTemplateRequest) {
		req.Name = "test-template"
	})
	workspace := coderdtest.CreateWorkspace(t, rootClient, template.ID, func(req *codersdk.CreateWorkspaceRequest) {
		req.Name = "test-workspace"
	})
	workspaceBuild := coderdtest.AwaitWorkspaceBuildJobCompleted(t, rootClient, workspace.LatestBuild.ID)

	replacements := map[string]string{
		firstUser.UserID.String():            pad("[first user ID]", 36),
		secondUser.ID.String():               pad("[second user ID]", 36),
		firstUser.OrganizationID.String():    pad("[first org ID]", 36),
		version.ID.String():                  pad("[version ID]", 36),
		version.Name:                         pad("[version name]", 36),
		version.Job.ID.String():              pad("[version job ID]", 36),
		version.Job.FileID.String():          pad("[version file ID]", 36),
		version.Job.WorkerID.String():        pad("[version worker ID]", 36),
		template.ID.String():                 pad("[template ID]", 36),
		workspace.ID.String():                pad("[workspace ID]", 36),
		workspaceBuild.ID.String():           pad("[workspace build ID]", 36),
		workspaceBuild.Job.ID.String():       pad("[workspace build job ID]", 36),
		workspaceBuild.Job.FileID.String():   pad("[workspace build file ID]", 36),
		workspaceBuild.Job.WorkerID.String(): pad("[workspace build worker ID]", 36),
	}

	return rootClient, replacements
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	n -= len(s)
	pre := n / 2
	post := n - pre
	return strings.Repeat("=", pre) + s + strings.Repeat("=", post)
}
