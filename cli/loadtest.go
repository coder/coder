package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/loadtest/harness"
)

func loadtest() *cobra.Command {
	var (
		configPath string
	)
	cmd := &cobra.Command{
		Use:   "loadtest --config <path>",
		Short: "Load test the Coder API",
		// TODO: documentation and a JSON scheme file
		Long: "Perform load tests against the Coder server. The load tests " +
			"configurable via a JSON file.",
		Hidden: true,
		Args:   cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				return xerrors.New("config is required")
			}

			var (
				configReader io.ReadCloser
			)
			if configPath == "-" {
				configReader = io.NopCloser(cmd.InOrStdin())
			} else {
				f, err := os.Open(configPath)
				if err != nil {
					return xerrors.Errorf("open config file %q: %w", configPath, err)
				}
				configReader = f
			}

			var config LoadTestConfig
			err := json.NewDecoder(configReader).Decode(&config)
			_ = configReader.Close()
			if err != nil {
				return xerrors.Errorf("read config file %q: %w", configPath, err)
			}

			err = config.Validate()
			if err != nil {
				return xerrors.Errorf("validate config: %w", err)
			}

			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			me, err := client.User(cmd.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("fetch current user: %w", err)
			}

			// Only owners can do loadtests. This isn't a very strong check but
			// there's not much else we can do. Ratelimits are enforced for
			// non-owners so hopefully that limits the damage if someone
			// disables this check and runs it against a non-owner account.
			ok := false
			for _, role := range me.Roles {
				if role.Name == "owner" {
					ok = true
					break
				}
			}
			if !ok {
				return xerrors.Errorf("Not logged in as site owner. Load testing is only available to site owners.")
			}

			// Disable ratelimits for future requests.
			client.BypassRatelimits = true

			// Prepare the test.
			strategy := config.Strategy.ExecutionStrategy()
			th := harness.NewTestHarness(strategy)

			for i, t := range config.Tests {
				name := fmt.Sprintf("%s-%d", t.Type, i)

				for j := 0; j < t.Count; j++ {
					id := strconv.Itoa(j)
					runner, err := t.NewRunner(client)
					if err != nil {
						return xerrors.Errorf("create %q runner for %s/%s: %w", t.Type, name, id, err)
					}

					th.AddRun(name, id, runner)
				}
			}

			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Running load test...")

			testCtx := cmd.Context()
			if config.Timeout > 0 {
				var cancel func()
				testCtx, cancel = context.WithTimeout(testCtx, time.Duration(config.Timeout))
				defer cancel()
			}

			// TODO: live progress output
			start := time.Now()
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}
			elapsed := time.Since(start)

			// Print the results.
			// TODO: better result printing
			// TODO: move result printing to the loadtest package, add multiple
			//       output formats (like HTML, JSON)
			res := th.Results()
			var totalDuration time.Duration
			for _, run := range res.Runs {
				totalDuration += run.Duration
				if run.Error == nil {
					continue
				}

				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n== FAIL: %s\n\n", run.FullID)
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\tError: %s\n\n", run.Error)

				// Print log lines indented.
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\tLog:\n")
				rd := bufio.NewReader(bytes.NewBuffer(run.Logs))
				for {
					line, err := rd.ReadBytes('\n')
					if err == io.EOF {
						break
					}
					if err != nil {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n\tLOG PRINT ERROR: %+v\n", err)
					}

					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\t\t%s", line)
				}
			}

			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "\n\nTest results:")
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\tPass:  %d\n", res.TotalPass)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\tFail:  %d\n", res.TotalFail)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\tTotal: %d\n", res.TotalRuns)
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "")
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\tTotal duration: %s\n", elapsed)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\tAvg. duration:  %s\n", totalDuration/time.Duration(res.TotalRuns))

			// Cleanup.
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "\nCleaning up...")
			err = th.Cleanup(cmd.Context())
			if err != nil {
				return xerrors.Errorf("cleanup tests: %w", err)
			}

			return nil
		},
	}

	cliflag.StringVarP(cmd.Flags(), &configPath, "config", "", "CODER_LOADTEST_CONFIG_PATH", "", "Path to the load test configuration file, or - to read from stdin.")
	return cmd
}
