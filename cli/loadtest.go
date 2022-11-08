package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/loadtest/harness"
)

func loadtest() *cobra.Command {
	var (
		configPath  string
		outputSpecs []string
	)
	cmd := &cobra.Command{
		Use:   "loadtest --config <path> [--output json[:path]] [--output text[:path]]]",
		Short: "Load test the Coder API",
		// TODO: documentation and a JSON schema file
		Long: "Perform load tests against the Coder server. The load tests are configurable via a JSON file.",
		Example: formatExamples(
			example{
				Description: "Run a loadtest with the given configuration file",
				Command:     "coder loadtest --config path/to/config.json",
			},
			example{
				Description: "Run a loadtest, reading the configuration from stdin",
				Command:     "cat path/to/config.json | coder loadtest --config -",
			},
			example{
				Description: "Run a loadtest outputting JSON results instead",
				Command:     "coder loadtest --config path/to/config.json --output json",
			},
			example{
				Description: "Run a loadtest outputting JSON results to a file",
				Command:     "coder loadtest --config path/to/config.json --output json:path/to/results.json",
			},
			example{
				Description: "Run a loadtest outputting text results to stdout and JSON results to a file",
				Command:     "coder loadtest --config path/to/config.json --output text --output json:path/to/results.json",
			},
		),
		Hidden: true,
		Args:   cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := loadLoadTestConfigFile(configPath, cmd.InOrStdin())
			if err != nil {
				return err
			}
			outputs, err := parseLoadTestOutputs(outputSpecs)
			if err != nil {
				return err
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
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			// Print the results.
			res := th.Results()
			for _, output := range outputs {
				var (
					w = cmd.OutOrStdout()
					c io.Closer
				)
				if output.path != "-" {
					f, err := os.Create(output.path)
					if err != nil {
						return xerrors.Errorf("create output file: %w", err)
					}
					w, c = f, f
				}

				switch output.format {
				case loadTestOutputFormatText:
					res.PrintText(w)
				case loadTestOutputFormatJSON:
					err = json.NewEncoder(w).Encode(res)
					if err != nil {
						return xerrors.Errorf("encode JSON: %w", err)
					}
				}

				if c != nil {
					err = c.Close()
					if err != nil {
						return xerrors.Errorf("close output file: %w", err)
					}
				}
			}

			// Cleanup.
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "\nCleaning up...")
			err = th.Cleanup(cmd.Context())
			if err != nil {
				return xerrors.Errorf("cleanup tests: %w", err)
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cliflag.StringVarP(cmd.Flags(), &configPath, "config", "", "CODER_LOADTEST_CONFIG_PATH", "", "Path to the load test configuration file, or - to read from stdin.")
	cliflag.StringArrayVarP(cmd.Flags(), &outputSpecs, "output", "", "CODER_LOADTEST_OUTPUTS", []string{"text"}, "Output formats, see usage for more information.")
	return cmd
}

func loadLoadTestConfigFile(configPath string, stdin io.Reader) (LoadTestConfig, error) {
	if configPath == "" {
		return LoadTestConfig{}, xerrors.New("config is required")
	}

	var (
		configReader io.ReadCloser
	)
	if configPath == "-" {
		configReader = io.NopCloser(stdin)
	} else {
		f, err := os.Open(configPath)
		if err != nil {
			return LoadTestConfig{}, xerrors.Errorf("open config file %q: %w", configPath, err)
		}
		configReader = f
	}

	var config LoadTestConfig
	err := json.NewDecoder(configReader).Decode(&config)
	_ = configReader.Close()
	if err != nil {
		return LoadTestConfig{}, xerrors.Errorf("read config file %q: %w", configPath, err)
	}

	err = config.Validate()
	if err != nil {
		return LoadTestConfig{}, xerrors.Errorf("validate config: %w", err)
	}

	return config, nil
}

type loadTestOutputFormat string

const (
	loadTestOutputFormatText loadTestOutputFormat = "text"
	loadTestOutputFormatJSON loadTestOutputFormat = "json"
	// TODO: html format
)

type loadTestOutput struct {
	format loadTestOutputFormat
	// Up to one path (the first path) will have the value "-" which signifies
	// stdout.
	path string
}

func parseLoadTestOutputs(outputs []string) ([]loadTestOutput, error) {
	var stdoutFormat loadTestOutputFormat

	validFormats := map[loadTestOutputFormat]struct{}{
		loadTestOutputFormatText: {},
		loadTestOutputFormatJSON: {},
	}

	var out []loadTestOutput
	for i, o := range outputs {
		parts := strings.SplitN(o, ":", 2)
		format := loadTestOutputFormat(parts[0])
		if _, ok := validFormats[format]; !ok {
			return nil, xerrors.Errorf("invalid output format %q in output flag %d", parts[0], i)
		}

		if len(parts) == 1 {
			if stdoutFormat != "" {
				return nil, xerrors.Errorf("multiple output flags specified for stdout")
			}
			stdoutFormat = format
			continue
		}
		if len(parts) != 2 {
			return nil, xerrors.Errorf("invalid output flag %d: %q", i, o)
		}

		out = append(out, loadTestOutput{
			format: format,
			path:   parts[1],
		})
	}

	// Default to --output text
	if stdoutFormat == "" && len(out) == 0 {
		stdoutFormat = loadTestOutputFormatText
	}

	if stdoutFormat != "" {
		out = append([]loadTestOutput{{
			format: stdoutFormat,
			path:   "-",
		}}, out...)
	}

	return out, nil
}
