package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/loadtest/harness"
	"github.com/coder/coder/loadtest/placebo"
	"github.com/coder/coder/loadtest/workspacebuild"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestLoadTest(t *testing.T) {
	t.Parallel()

	t.Run("PlaceboFromStdin", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		config := cli.LoadTestConfig{
			Strategy: cli.LoadTestStrategy{
				Type: cli.LoadTestStrategyTypeLinear,
			},
			Tests: []cli.LoadTest{
				{
					Type:  cli.LoadTestTypePlacebo,
					Count: 10,
					Placebo: &placebo.Config{
						Sleep: httpapi.Duration(10 * time.Millisecond),
					},
				},
			},
			Timeout: httpapi.Duration(testutil.WaitShort),
		}

		configBytes, err := json.Marshal(config)
		require.NoError(t, err)

		cmd, root := clitest.New(t, "loadtest", "--config", "-")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(bytes.NewReader(configBytes))
		cmd.SetOut(pty.Output())
		cmd.SetErr(pty.Output())

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		done := make(chan any)
		go func() {
			errC := cmd.ExecuteContext(ctx)
			assert.NoError(t, errC)
			close(done)
		}()
		pty.ExpectMatch("Test results:")
		pty.ExpectMatch("Pass:  10")
		cancelFunc()
		<-done
	})

	t.Run("WorkspaceBuildFromFile", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		config := cli.LoadTestConfig{
			Strategy: cli.LoadTestStrategy{
				Type:             cli.LoadTestStrategyTypeConcurrent,
				ConcurrencyLimit: 2,
			},
			Tests: []cli.LoadTest{
				{
					Type:  cli.LoadTestTypeWorkspaceBuild,
					Count: 2,
					WorkspaceBuild: &workspacebuild.Config{
						OrganizationID: user.OrganizationID,
						UserID:         user.UserID.String(),
						Request: codersdk.CreateWorkspaceRequest{
							TemplateID: template.ID,
						},
					},
				},
			},
			Timeout: httpapi.Duration(testutil.WaitLong),
		}

		d := t.TempDir()
		configPath := filepath.Join(d, "/config.loadtest.json")
		f, err := os.Create(configPath)
		require.NoError(t, err)
		defer f.Close()
		err = json.NewEncoder(f).Encode(config)
		require.NoError(t, err)
		_ = f.Close()

		cmd, root := clitest.New(t, "loadtest", "--config", configPath)
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		cmd.SetErr(pty.Output())

		ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancelFunc()

		done := make(chan any)
		go func() {
			errC := cmd.ExecuteContext(ctx)
			assert.NoError(t, errC)
			close(done)
		}()
		pty.ExpectMatch("Test results:")
		pty.ExpectMatch("Pass:  2")
		<-done
		cancelFunc()
	})

	t.Run("OutputFormats", func(t *testing.T) {
		t.Parallel()

		type outputFlag struct {
			format string
			path   string
		}

		dir := t.TempDir()

		cases := []struct {
			name        string
			outputs     []outputFlag
			errContains string
		}{
			{
				name:    "Default",
				outputs: []outputFlag{},
			},
			{
				name:    "ExplicitText",
				outputs: []outputFlag{{format: "text"}},
			},
			{
				name: "JSON",
				outputs: []outputFlag{
					{
						format: "json",
						path:   filepath.Join(dir, "results.json"),
					},
				},
			},
			{
				name: "TextAndJSON",
				outputs: []outputFlag{
					{
						format: "text",
					},
					{
						format: "json",
						path:   filepath.Join(dir, "results.json"),
					},
				},
			},
			{
				name: "TextAndJSON2",
				outputs: []outputFlag{
					{
						format: "text",
					},
					{
						format: "text",
						path:   filepath.Join(dir, "results.txt"),
					},
					{
						format: "json",
						path:   filepath.Join(dir, "results.json"),
					},
				},
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				client := coderdtest.New(t, nil)
				_ = coderdtest.CreateFirstUser(t, client)

				config := cli.LoadTestConfig{
					Strategy: cli.LoadTestStrategy{
						Type: cli.LoadTestStrategyTypeLinear,
					},
					Tests: []cli.LoadTest{
						{
							Type:  cli.LoadTestTypePlacebo,
							Count: 10,
							Placebo: &placebo.Config{
								Sleep: httpapi.Duration(10 * time.Millisecond),
							},
						},
					},
					Timeout: httpapi.Duration(testutil.WaitShort),
				}

				configBytes, err := json.Marshal(config)
				require.NoError(t, err)

				args := []string{"loadtest", "--config", "-"}
				for _, output := range c.outputs {
					flag := output.format
					if output.path != "" {
						flag += ":" + output.path
					}
					args = append(args, "--output", flag)
				}

				cmd, root := clitest.New(t, args...)
				clitest.SetupConfig(t, client, root)
				cmd.SetIn(bytes.NewReader(configBytes))
				out := bytes.NewBuffer(nil)
				cmd.SetOut(out)
				pty := ptytest.New(t)
				cmd.SetErr(pty.Output())

				ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancelFunc()

				done := make(chan any)
				go func() {
					errC := cmd.ExecuteContext(ctx)
					if c.errContains != "" {
						assert.Error(t, errC)
						assert.Contains(t, errC.Error(), c.errContains)
					} else {
						assert.NoError(t, errC)
					}
					close(done)
				}()

				<-done

				if c.errContains != "" {
					return
				}
				if len(c.outputs) == 0 {
					// This is the default output format when no flags are
					// specified.
					c.outputs = []outputFlag{{format: "text"}}
				}
				for i, output := range c.outputs {
					msg := fmt.Sprintf("flag %d", i)
					var b []byte
					if output.path == "" {
						b = out.Bytes()
					} else {
						b, err = os.ReadFile(output.path)
						require.NoError(t, err, msg)
					}

					switch output.format {
					case "text":
						require.Contains(t, string(b), "Test results:", msg)
						require.Contains(t, string(b), "Pass:  10", msg)
					case "json":
						var res harness.Results
						err = json.Unmarshal(b, &res)
						require.NoError(t, err, msg)
						require.Equal(t, 10, res.TotalRuns, msg)
						require.Equal(t, 10, res.TotalPass, msg)
						require.Len(t, res.Runs, 10, msg)
					}
				}
			})
		}
	})
}
