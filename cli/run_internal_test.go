package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

func TestRunCLIParsingErrors(t *testing.T) {
	t.Parallel()
	for _, verbose := range []bool{false, true} {
		for _, tc := range []struct {
			name     string
			args     []string
			rootFlag bool
			rootArgs []string
		}{
			{name: "EnvSeparateValue", args: []string{"--env", "TOKEN=REVIEW_SECRET_SENTINEL", "--not-a-run-flag"}},
			{name: "EnvEqualsValue", args: []string{"--env=TOKEN=REVIEW_SECRET_SENTINEL", "--not-a-run-flag"}},
			{name: "EnvUnparsedValue", args: []string{"--not-a-run-flag", "--env", "TOKEN=REVIEW_SECRET_SENTINEL"}},
			{name: "EnvUnknownRootFlag", args: []string{"--env", "TOKEN=REVIEW_SECRET_SENTINEL"}, rootFlag: true},
			{name: "SeparateValue", args: []string{"--cmd", "echo REVIEW_SECRET_SENTINEL", "--not-a-run-flag"}},
			{name: "EqualsValue", args: []string{"--cmd=echo REVIEW_SECRET_SENTINEL", "--not-a-run-flag"}},
			{name: "UnparsedValue", args: []string{"--not-a-run-flag", "--cmd", "echo REVIEW_SECRET_SENTINEL"}},
			{name: "RepeatedValue", args: []string{"--cmd", "echo REVIEW_SECRET_SENTINEL", "--cmd", "true", "--not-a-run-flag"}},
			{name: "UnknownRootFlag", args: []string{"--cmd", "echo REVIEW_SECRET_SENTINEL"}, rootFlag: true},
			{name: "UnknownRootFlagEqualsValue", args: []string{"--cmd=echo REVIEW_SECRET_SENTINEL"}, rootFlag: true},
			{name: "UnknownRootFlagRepeatedValue", args: []string{"--cmd", "echo REVIEW_SECRET_SENTINEL", "--cmd", "true"}, rootFlag: true},
			{name: "RootValueLooksLikeSeparator", rootArgs: []string{"--header", "--"}, args: []string{"--cmd", "echo REVIEW_SECRET_SENTINEL"}, rootFlag: true},
			{name: "RunValueLooksLikeSeparator", args: []string{"--template", "--", "--cmd", "echo REVIEW_SECRET_SENTINEL"}, rootFlag: true},
			{name: "ShortValueLooksLikeSeparator", args: []string{"-O", "--", "--cmd", "echo REVIEW_SECRET_SENTINEL"}, rootFlag: true},
			{name: "ClusteredShortValueLooksLikeSeparator", args: []string{"-vO", "--", "--cmd", "echo REVIEW_SECRET_SENTINEL"}, rootFlag: true},
		} {
			t.Run(fmt.Sprintf("%s/Verbose%t", tc.name, verbose), func(t *testing.T) {
				t.Parallel()
				var args []string
				if verbose {
					args = append(args, "--verbose")
				}
				commandName, unknownFlag := "coder run", "--not-a-run-flag"
				if tc.rootFlag {
					commandName, unknownFlag = "coder", "--not-a-root-flag"
					args = append(args, unknownFlag)
				}
				args = append(args, tc.rootArgs...)
				args = append(args, "run", "--template", "prepared", "--cpu", "1", "--ram", "16MiB")
				args = append(args, tc.args...)
				stderr := runCLIError(t, args...)
				require.NotContains(t, stderr, "REVIEW_SECRET_SENTINEL")
				require.Contains(t, stderr, fmt.Sprintf("parsing flags for %q", commandName))
				require.Contains(t, stderr, "unknown flag: "+unknownFlag)
			})
		}
	}
}

// This helper runs the actual CLI error and exit path in an isolated process.
func TestRunCLIErrorHelper(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_RUN_CLI_ERROR_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"coder"}, os.Args[i+1:]...)
			var root RootCmd
			root.RunWithSubcommands(root.CoreSubcommands())
			return
		}
	}
	t.Fatal("missing CLI argument separator")
}

func TestRunParsingErrorFormattingScope(t *testing.T) {
	t.Parallel()
	for _, verbose := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			args []string
		}{
			{name: "OtherCommand", args: []string{"version", "--not-a-version-flag", "retained-argument-context"}},
			{name: "BeforeOtherCommand", args: []string{"--not-a-version-flag", "version", "retained-argument-context"}},
			{name: "RunAsArgument", args: []string{"--not-a-version-flag", "version", "run", "retained-argument-context"}},
			{name: "AfterSeparator", args: []string{"--not-a-version-flag", "version", "--", "--cmd", "retained-argument-context"}},
			{name: "FlagValueLooksSensitive", args: []string{"--not-a-version-flag", "--header", "--cmd", "version", "retained-argument-context"}},
			{name: "FlagValueLooksSensitiveEquals", args: []string{"--not-a-version-flag", "--header", "--cmd=literal", "version", "retained-argument-context"}},
		} {
			t.Run(fmt.Sprintf("%s/Verbose%t", tc.name, verbose), func(t *testing.T) {
				t.Parallel()
				var args []string
				if verbose {
					args = append(args, "--verbose")
				}
				args = append(args, tc.args...)
				stderr := runCLIError(t, args...)
				require.Contains(t, stderr, "retained-argument-context")
				require.Contains(t, stderr, "unknown flag: --not-a-version-flag")
			})
		}
	}
}

func runCLIError(t *testing.T, args ...string) string {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	args = append([]string{"-test.run=^TestRunCLIErrorHelper$", "--", "--global-config", t.TempDir()}, args...)
	command := exec.CommandContext(testutil.Context(t, testutil.WaitLong), executable, args...)
	command.Env = append(os.Environ(), "CODER_RUN_CLI_ERROR_HELPER=1")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 1, exitErr.ExitCode())
	require.Empty(t, stdout.String())
	return stderr.String()
}

func TestRunRequiredFlags(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { assert.Zero(t, requests.Load(), "invalid input must not contact the API") })

	flags := []string{"template", "cpu", "ram", "cmd"}
	values := []string{"prepared", "2", "512MiB", "echo secret-command"}
	for mask := 0; mask < 15; mask++ {
		t.Run(fmt.Sprintf("Present%04b", mask), func(t *testing.T) {
			t.Parallel()
			var args []string
			for i, flag := range flags {
				if mask&(1<<i) != 0 {
					args = append(args, "--"+flag, values[i])
				}
			}
			inv, stdout, stderr := newRunTestInvocation(t, server.URL, quartz.NewMock(t), args...)
			inv.Environ = serpent.ParseEnviron([]string{
				"CODER_TEMPLATE_NAME=prepared", "CODER_CPU=2", "CODER_RAM=512MiB", "CODER_CMD=echo secret-command",
			}, "")
			err := inv.Run()
			require.ErrorContains(t, err, "required flags")
			for i, flag := range flags {
				if mask&(1<<i) == 0 {
					require.ErrorContains(t, err, flag)
				}
			}
			require.NotContains(t, err.Error(), "secret-command")
			require.Empty(t, stdout.String())
			require.Empty(t, stderr.String())
		})
	}
}

func TestRunInvalidInput(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { assert.Zero(t, requests.Load()) })

	for _, tc := range []struct {
		name, flag, value, want string
	}{
		{"EmptyTemplate", "template", "", "--template"},
		{"BlankTemplate", "template", "  ", "--template"},
		{"EmptyCPU", "cpu", "", "--cpu"},
		{"ZeroCPU", "cpu", "0", "--cpu"},
		{"NegativeCPU", "cpu", "-2", "--cpu"},
		{"FractionalCPU", "cpu", "1.5", "--cpu"},
		{"Millicores", "cpu", "500m", "--cpu"},
		{"OverflowCPU", "cpu", "9999999999999999999999", "--cpu"},
		{"EmptyRAM", "ram", "", "--ram"},
		{"TooLittleRAM", "ram", "15MiB", "--ram"},
		{"ZeroRAM", "ram", "0MiB", "--ram"},
		{"NegativeRAM", "ram", "-1GiB", "--ram"},
		{"FractionalRAM", "ram", "1.5GiB", "--ram"},
		{"DecimalRAM", "ram", "512MB", "--ram"},
		{"UnitlessRAM", "ram", "512", "--ram"},
		{"OverflowRAM", "ram", "9999999999999999999999GiB", "--ram"},
		{"EnvWithoutEquals", "env", "secret-command", "--env requires KEY=value"},
		{"EmptyCommand", "cmd", "", "cmd must be nonempty"},
		{"BlankCommand", "cmd", " \n\t", "cmd must be nonempty"},
		{"NULCommand", "cmd", "echo secret-command\x00", "contain no NUL"},
		{"LongCommand", "cmd", strings.Repeat("x", (64<<10)+1), "at most 64KiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			values := map[string]string{"template": "prepared", "cpu": "2", "ram": "512MiB", "cmd": "echo secret-command"}
			values[tc.flag] = tc.value
			var args []string
			for _, flag := range []string{"template", "cpu", "ram", "cmd", "env"} {
				if value, ok := values[flag]; ok {
					args = append(args, "--"+flag, value)
				}
			}
			inv, stdout, stderr := newRunTestInvocation(t, server.URL, quartz.NewMock(t), args...)
			err := inv.Run()
			require.ErrorContains(t, err, tc.want)
			require.NotContains(t, err.Error(), "secret-command")
			require.Empty(t, stdout.String())
			require.Empty(t, stderr.String())
		})
	}
}

func TestRunTemplateResolution(t *testing.T) {
	t.Parallel()
	org1 := codersdk.Organization{MinimalOrganization: codersdk.MinimalOrganization{ID: uuid.New(), Name: "first"}}
	org2 := codersdk.Organization{MinimalOrganization: codersdk.MinimalOrganization{ID: uuid.New(), Name: "second"}}
	tpl1 := codersdk.Template{ID: uuid.New(), Name: "prepared", OrganizationID: org1.ID, OrganizationName: org1.Name}
	tpl2 := codersdk.Template{ID: uuid.New(), Name: "prepared", OrganizationID: org2.ID, OrganizationName: org2.Name}
	for _, tc := range []struct {
		name      string
		selector  string
		templates []codersdk.Template
		orgFlag   string
		orgEnv    string
		want      codersdk.Template
		wantErr   string
	}{
		{name: "UniqueName", selector: "prepared", templates: []codersdk.Template{tpl1}, want: tpl1},
		{name: "UUID", selector: tpl1.ID.String(), want: tpl1},
		{name: "Ambiguous", selector: "prepared", templates: []codersdk.Template{tpl1, tpl2}, wantErr: "use --org="},
		{name: "OrganizationName", selector: "prepared", templates: []codersdk.Template{tpl1, tpl2}, orgFlag: org2.Name, want: tpl2},
		{name: "OrganizationUUID", selector: "prepared", templates: []codersdk.Template{tpl1, tpl2}, orgFlag: org2.ID.String(), want: tpl2},
		{name: "OrganizationEnv", selector: "prepared", templates: []codersdk.Template{tpl1, tpl2}, orgEnv: org2.Name, want: tpl2},
		{name: "UniqueNameWrongOrg", selector: "prepared", templates: []codersdk.Template{tpl1}, orgFlag: org2.Name, wantErr: "not in organization"},
		{name: "UUIDWrongOrg", selector: tpl1.ID.String(), orgFlag: org2.Name, wantErr: "not in organization"},
		{name: "NotFound", selector: "prepared", wantErr: "no template found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var creates atomic.Int32
			workload := codersdk.Workload{State: "succeeded", ExitCode: new(0)}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v2/templates", func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, `exact_name:"prepared"`, r.URL.Query().Get("q"))
				writeRunTestJSON(t, w, tc.templates)
			})
			mux.HandleFunc("GET /api/v2/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tpl1.ID.String(), r.PathValue("id"))
				writeRunTestJSON(t, w, tpl1)
			})
			mux.HandleFunc("GET /api/v2/users/me/organizations", func(w http.ResponseWriter, _ *http.Request) {
				writeRunTestJSON(t, w, []codersdk.Organization{org1, org2})
			})
			mux.HandleFunc("POST /api/v2/organizations/{org}/workloads", func(w http.ResponseWriter, r *http.Request) {
				creates.Add(1)
				var req codersdk.CreateWorkloadRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				assert.NoError(t, req.Validate())
				assert.Equal(t, tc.want.ID, req.TemplateID)
				assert.Equal(t, tc.want.OrganizationID.String(), r.PathValue("org"))
				assert.Equal(t, int64(1), req.CPU)
				assert.Equal(t, int64(16<<20), req.MemoryBytes)
				workload.ID = req.ID
				writeRunTestJSON(t, w, workload)
			})
			mux.HandleFunc("GET /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, _ *http.Request) {
				writeRunTestStream(t, w, codersdk.WorkloadOutput{Workload: workload})
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			args := []string{"--template", tc.selector, "--cpu", "1", "--ram", "16MiB", "--cmd", "true"}
			if tc.orgFlag != "" {
				args = append(args, "--org", tc.orgFlag)
			}
			inv, stdout, _ := newRunTestInvocation(t, server.URL, quartz.NewMock(t), args...)
			if tc.orgEnv != "" {
				inv.Environ = serpent.ParseEnviron([]string{"CODER_ORGANIZATION=" + tc.orgEnv}, "")
			}
			err := inv.Run()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Zero(t, creates.Load())
			} else {
				require.NoError(t, err)
				require.EqualValues(t, 1, creates.Load())
			}
			require.Empty(t, stdout.String())
		})
	}
}

func TestRunStreamingAndExit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		exitCode int
		flag     string
		lookup   string
	}{
		{exitCode: 0},
		{exitCode: 23},
		{exitCode: 137},
		{exitCode: 0, flag: "-v"},
		{exitCode: 23, flag: "--verbose"},
		{exitCode: 137, flag: "-v"},
		{exitCode: 23, flag: "-v", lookup: "forbidden"},
		{exitCode: 0, flag: "-v", lookup: "hidden"},
	} {
		t.Run(fmt.Sprintf("Exit%d/%s/%s", tc.exitCode, tc.flag, tc.lookup), func(t *testing.T) {
			t.Parallel()
			exitCode := tc.exitCode
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := quartz.NewMock(t)
			trap := clock.Trap().NewTimer("workload-reconnect")
			defer trap.Close()
			template := codersdk.Template{ID: uuid.New(), OrganizationID: uuid.New()}
			workload := codersdk.Workload{
				State: "succeeded", ExitCode: &exitCode, AgentID: uuid.New(),
				Timings: codersdk.WorkloadTimings{
					HostConnectNS: new(int64(2 * time.Millisecond)), HostConnectionReused: new(true),
					ProvisionNS: new(int64(15 * time.Millisecond)), ExecNS: new(int64(60 * time.Millisecond)),
				},
				Generation: codersdk.WorkloadGeneration{
					ID: "immutable-generation", Workdir: "/prepared/project",
					Repositories: map[string]string{
						"zeta":  strings.Repeat("a", 40),
						"alpha": strings.Repeat("b", 40),
					},
				},
			}
			if exitCode != 0 {
				workload.State = "failed"
			}
			var creates, polls, cancels, lookups atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v2/buildinfo", func(w http.ResponseWriter, _ *http.Request) {
				writeRunTestJSON(t, w, codersdk.BuildInfoResponse{})
			})
			mux.HandleFunc("GET /api/v2/workspaces", func(w http.ResponseWriter, r *http.Request) {
				lookup := lookups.Add(1)
				assert.Contains(t, r.URL.Query().Get("q"), template.OrganizationID.String())
				if tc.lookup == "forbidden" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if tc.lookup == "hidden" {
					writeRunTestJSON(t, w, codersdk.WorkspacesResponse{})
					return
				}
				workspace := codersdk.Workspace{
					OwnerName: "admin", Name: "hypervisor",
					LatestBuild: codersdk.WorkspaceBuild{Resources: []codersdk.WorkspaceResource{{
						Agents: []codersdk.WorkspaceAgent{{ID: workload.AgentID}},
					}}},
				}
				if lookup == 1 {
					// Do not mistake the first workspace for the selected host.
					workspace.Name = "other-host"
					workspace.LatestBuild.Resources[0].Agents[0].ID = uuid.New()
				} else {
					assert.Equal(t, "1", r.URL.Query().Get("offset"))
				}
				writeRunTestJSON(t, w, codersdk.WorkspacesResponse{Workspaces: []codersdk.Workspace{workspace}, Count: 2})
			})
			mux.HandleFunc("GET /api/v2/templates/{id}", func(w http.ResponseWriter, _ *http.Request) {
				clock.Advance(10 * time.Millisecond).MustWait(ctx)
				w.Header().Set(codersdk.BuildVersionHeader, "v999.0.0")
				w.Header().Set(codersdk.EntitlementsWarningHeader, "test feature warning")
				writeRunTestJSON(t, w, template)
			})
			mux.HandleFunc("POST /api/v2/organizations/{org}/workloads", func(w http.ResponseWriter, r *http.Request) {
				creates.Add(1)
				var req codersdk.CreateWorkloadRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				assert.NoError(t, req.Validate())
				assert.Equal(t, template.ID, req.TemplateID)
				assert.Equal(t, template.OrganizationID.String(), r.PathValue("org"))
				assert.Equal(t, int64(3), req.CPU)
				assert.Equal(t, int64(2<<30), req.MemoryBytes)
				assert.Equal(t, "echo secret-command", req.Command)
				assert.Equal(t, map[string]string{"TOKEN": "secret-env", "EMPTY": "", "TEXT": "a,b=c\n\"quoted\"", "DUPLICATE": "last"}, req.Env)
				clock.Advance(20 * time.Millisecond).MustWait(ctx)
				workload.ID = req.ID
				writeRunTestJSON(t, w, workload)
			})
			mux.HandleFunc("GET /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, r *http.Request) {
				connection := polls.Add(1)
				assert.Equal(t, workload.ID.String(), r.PathValue("id"))
				assert.Equal(t, template.OrganizationID.String(), r.PathValue("org"))
				assert.Equal(t, "true", r.URL.Query().Get("follow"))
				output := codersdk.WorkloadOutput{Workload: workload}
				switch connection {
				case 1:
					clock.Advance(5 * time.Millisecond).MustWait(ctx)
					assert.Equal(t, "0", r.URL.Query().Get("offset"))
					output.Workload.State = "running"
					output.Events = []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte{'a', 0, 0xff, '\n'}}}
					output.NextOffset = 1
					writeRunTestStream(t, w, output)
					// A partial frame must not advance the resume offset.
					_, _ = io.WriteString(w, `{"workload":`)
				case 2:
					assert.Equal(t, "1", r.URL.Query().Get("offset"))
					w.WriteHeader(http.StatusBadGateway)
				default:
					assert.EqualValues(t, 3, connection)
					assert.Equal(t, "1", r.URL.Query().Get("offset"))
					output.Events = []codersdk.WorkloadEvent{
						{Stream: "stderr", Data: []byte("command stderr\n")},
						{Stream: "stdout", Data: []byte("more stdout\n")},
					}
					output.NextOffset = 3
					writeRunTestStream(t, w, output)
					output.Events = []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("last stdout")}}
					output.NextOffset = 4
					writeRunTestStream(t, w, output)
					output.Events = nil
					writeRunTestStream(t, w, output)
				}
			})
			mux.HandleFunc("DELETE /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, _ *http.Request) {
				cancels.Add(1)
				w.WriteHeader(http.StatusNoContent)
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			args := []string{"--template", template.ID.String(), "--cpu", "3", "--ram", "2GiB", "--cmd", "echo secret-command", "--debug-http",
				"--env", "TOKEN=secret-env", "--env=EMPTY=", "--env", "TEXT=a,b=c\n\"quoted\"", "--env", "DUPLICATE=first", "--env", "DUPLICATE=last"}
			if tc.flag != "" {
				args = append(args, tc.flag)
			}
			inv, stdout, stderr := newRunTestInvocation(t, server.URL, clock, args...)
			done := make(chan error, 1)
			go func() { done <- inv.WithContext(ctx).Run() }()
			for _, delay := range []time.Duration{50 * time.Millisecond, 100 * time.Millisecond} {
				call := trap.MustWait(ctx)
				require.Equal(t, delay, call.Duration)
				call.MustRelease(ctx)
				clock.Advance(delay).MustWait(ctx)
			}
			err := testutil.TryReceive(ctx, t, done)
			if exitCode == 0 {
				require.NoError(t, err)
			} else {
				var exitErr *exitError
				require.ErrorAs(t, err, &exitErr)
				require.Equal(t, exitCode, exitErr.ExitCode())
				require.Nil(t, exitErr.Unwrap(), "command exits must not print a CLI error")
			}
			require.Equal(t, []byte("a\x00\xff\nmore stdout\nlast stdout"), stdout.Bytes())
			if tc.flag == "" {
				require.Equal(t, "command stderr\n", stderr.String())
				require.Zero(t, lookups.Load(), "quiet runs must not look up workspace names")
			} else {
				require.Contains(t, stderr.String(), "test feature warning")
				require.Contains(t, stderr.String(), fmt.Sprintf("\nTimings (%% of total):\n  Provision      15ms (8.1%%)\n  Exec           60ms (32.4%%)\n  Overhead       110ms (59.5%%)\n  Total          185ms\nLatencies:\n  Dispatch       30ms\n  First output   35ms\nExit code: %d\n", exitCode))
				if tc.lookup == "" {
					require.Contains(t, stderr.String(), fmt.Sprintf("Attached to workspace: admin/hypervisor (agent: %s)\n", workload.AgentID))
					require.EqualValues(t, 2, lookups.Load())
				} else {
					require.Contains(t, stderr.String(), fmt.Sprintf("Attached to agent: %s (workspace name unavailable:", workload.AgentID))
					require.EqualValues(t, 1, lookups.Load())
				}
				require.Contains(t, stderr.String(), fmt.Sprintf("Workload: %s\nGeneration: immutable-generation\nWorkdir: %q\nRepository: %q @ %s\nRepository: %q @ %s\nConnection interrupted; reconnecting to workload %s.\ncommand stderr\n",
					workload.ID, workload.Generation.Workdir, "alpha", strings.Repeat("b", 40), "zeta", strings.Repeat("a", 40), workload.ID))
			}
			require.NotContains(t, stderr.String(), "secret-command")
			require.NotContains(t, stderr.String(), "secret-env")
			require.EqualValues(t, 1, creates.Load())
			require.EqualValues(t, 3, polls.Load())
			require.Zero(t, cancels.Load())
		})
	}
}

func TestRunTimingSummary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                                             string
		timings                                          codersdk.WorkloadTimings
		total                                            time.Duration
		firstOutput                                      *time.Duration
		wantProvision, wantExec, wantOverhead, wantFirst string
		wantPercentages                                  bool
	}{
		{
			name: "Percentages",
			timings: codersdk.WorkloadTimings{
				HostConnectNS: new(int64(1234567)), HostConnectionReused: new(false),
				ProvisionNS: new(int64(2500000)), ExecNS: new(int64(0)),
			},
			total: 10 * time.Millisecond, firstOutput: new(4 * time.Millisecond),
			wantProvision: "2.5ms (25.0%)", wantExec: "0s (0.0%)", wantOverhead: "7.5ms (75.0%)", wantFirst: "4ms",
			wantPercentages: true,
		},
		{
			name:    "OverlappingDispatch",
			timings: codersdk.WorkloadTimings{ProvisionNS: new(int64(3 * time.Millisecond)), ExecNS: new(int64(6 * time.Millisecond))},
			total:   10 * time.Millisecond, firstOutput: new(4 * time.Millisecond),
			wantProvision: "3ms (30.0%)", wantExec: "6ms (60.0%)", wantOverhead: "1ms (10.0%)", wantFirst: "4ms",
			wantPercentages: true,
		},
		{
			name:          "RoundedPercentagesSumTo100",
			timings:       codersdk.WorkloadTimings{ProvisionNS: new(int64(time.Millisecond)), ExecNS: new(int64(time.Millisecond))},
			total:         3 * time.Millisecond,
			wantProvision: "1ms (33.4%)", wantExec: "1ms (33.3%)", wantOverhead: "1ms (33.3%)", wantFirst: "none",
			wantPercentages: true,
		},
		{
			name:          "NoOverhead",
			timings:       codersdk.WorkloadTimings{ProvisionNS: new(int64(5006000)), ExecNS: new(int64(4994000))},
			total:         10 * time.Millisecond,
			wantProvision: "5.006ms (50.1%)", wantExec: "4.994ms (49.9%)", wantOverhead: "0s (0.0%)", wantFirst: "none",
			wantPercentages: true,
		},
		{
			name: "OlderServerNoOutput", total: 10 * time.Millisecond,
			wantProvision: "unavailable", wantExec: "unavailable", wantOverhead: "unavailable", wantFirst: "none",
		},
		{
			name: "MissingExec", total: 10 * time.Millisecond,
			timings:       codersdk.WorkloadTimings{ProvisionNS: new(int64(2500000))},
			wantProvision: "2.5ms", wantExec: "unavailable", wantOverhead: "unavailable", wantFirst: "none",
		},
		{
			name: "InvalidDurations", total: 10 * time.Millisecond,
			timings:       codersdk.WorkloadTimings{ProvisionNS: new(int64(-1)), ExecNS: new(int64(-1))},
			wantProvision: "unavailable", wantExec: "unavailable", wantOverhead: "unavailable", wantFirst: "none",
		},
		{
			name: "HostDurationsExceedTotal", total: 10 * time.Millisecond,
			timings:       codersdk.WorkloadTimings{ProvisionNS: new(int64(6 * time.Millisecond)), ExecNS: new(int64(5 * time.Millisecond))},
			wantProvision: "6ms", wantExec: "5ms", wantOverhead: "unavailable", wantFirst: "none",
		},
		{
			name:          "ZeroTotal",
			timings:       codersdk.WorkloadTimings{ProvisionNS: new(int64(0)), ExecNS: new(int64(0))},
			wantProvision: "0s", wantExec: "0s", wantOverhead: "unavailable", wantFirst: "none",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			timings := workloadRunTimings{dispatch: 3 * time.Millisecond, firstOutput: tc.firstOutput}
			timings.print(&stderr, codersdk.Workload{Timings: tc.timings, ExitCode: new(23)}, tc.total)
			header := "Timings (breakdown unavailable):"
			if tc.wantPercentages {
				header = "Timings (% of total):"
			}
			require.Equal(t, fmt.Sprintf("\n%s\n  Provision      %s\n  Exec           %s\n  Overhead       %s\n  Total          %s\nLatencies:\n  Dispatch       3ms\n  First output   %s\nExit code: 23\n", header, tc.wantProvision, tc.wantExec, tc.wantOverhead, tc.total, tc.wantFirst), stderr.String())
		})
	}
}

func TestRunCancellation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		interrupt     bool
		duringCreate  bool
		cancelFailure bool
		verbose       bool
	}{
		{name: "SIGINT", interrupt: true},
		{name: "SIGINTVerbose", interrupt: true, verbose: true},
		{name: "ContextOnly"},
		{name: "SIGINTDuringCreate", interrupt: true, duringCreate: true},
		{name: "CancellationFailure", interrupt: true, cancelFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			invCtx, cancelInvocation := context.WithCancel(ctx)
			defer cancelInvocation()
			template := codersdk.Template{ID: uuid.New(), OrganizationID: uuid.New()}
			var creates, cancels atomic.Int32
			workload := codersdk.Workload{State: "running"}
			blocked := make(chan struct{}, 1)
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v2/templates/{id}", func(w http.ResponseWriter, _ *http.Request) {
				writeRunTestJSON(t, w, template)
			})
			mux.HandleFunc("POST /api/v2/organizations/{org}/workloads", func(w http.ResponseWriter, r *http.Request) {
				creates.Add(1)
				var req codersdk.CreateWorkloadRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				workload.ID = req.ID
				if tc.duringCreate {
					blocked <- struct{}{}
					<-r.Context().Done()
					return
				}
				writeRunTestJSON(t, w, workload)
			})
			mux.HandleFunc("GET /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, r *http.Request) {
				writeRunTestStream(t, w, codersdk.WorkloadOutput{Workload: workload})
				blocked <- struct{}{}
				<-r.Context().Done()
			})
			mux.HandleFunc("DELETE /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, r *http.Request) {
				cancels.Add(1)
				assert.NoError(t, r.Context().Err())
				assert.Equal(t, template.OrganizationID.String(), r.PathValue("org"))
				assert.Equal(t, workload.ID.String(), r.PathValue("id"))
				assert.Equal(t, "test-token", r.Header.Get(codersdk.SessionTokenHeader))
				if tc.cancelFailure {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			args := []string{"--template", template.ID.String(), "--cpu", "1", "--ram", "16MiB", "--cmd", "echo secret-command"}
			if tc.verbose {
				args = append(args, "-v")
			}
			inv, stdout, stderr := newRunTestInvocation(t, server.URL, quartz.NewMock(t), args...)
			interrupts := make(chan context.CancelFunc, 1)
			inv = inv.WithTestSignalNotifyContext(t, func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
				assert.Equal(t, []os.Signal{os.Interrupt}, signals)
				assert.Nil(t, parent.Done(), "invocation cancellation must not masquerade as SIGINT")
				signalCtx, cancel := context.WithCancel(parent)
				interrupts <- cancel
				return signalCtx, cancel
			})
			done := make(chan error, 1)
			go func() { done <- inv.WithContext(invCtx).Run() }()
			interrupt := testutil.TryReceive(ctx, t, interrupts)
			testutil.TryReceive(ctx, t, blocked)
			if tc.interrupt {
				interrupt()
			}
			// The cancellation request must work even after the invocation
			// and its active HTTP request have both been canceled.
			cancelInvocation()
			err := testutil.TryReceive(ctx, t, done)
			if tc.interrupt {
				var exitErr *exitError
				require.ErrorAs(t, err, &exitErr)
				require.Equal(t, 130, exitErr.ExitCode())
				if tc.cancelFailure {
					require.ErrorContains(t, err, "cancel workload")
				} else {
					require.Nil(t, exitErr.Unwrap())
					if tc.verbose {
						require.Contains(t, stderr.String(), "Cancellation requested")
					} else {
						require.Empty(t, stderr.String())
					}
				}
				require.EqualValues(t, 1, cancels.Load())
			} else {
				require.ErrorIs(t, err, context.Canceled)
				require.Zero(t, cancels.Load())
			}
			require.EqualValues(t, 1, creates.Load())
			require.Empty(t, stdout.String())
			require.NotContains(t, stderr.String(), "secret-command")
		})
	}
}

func TestRunCancellationWithBlockedOutput(t *testing.T) {
	t.Parallel()
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			org, id := uuid.New(), uuid.New()
			canceled := make(chan struct{}, 1)
			var cancellations atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v2/organizations/{org}/workloads", func(w http.ResponseWriter, _ *http.Request) {
				writeRunTestJSON(t, w, codersdk.Workload{ID: id, State: "running"})
			})
			mux.HandleFunc("GET /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, _ *http.Request) {
				writeRunTestStream(t, w, codersdk.WorkloadOutput{
					Workload:   codersdk.Workload{ID: id, State: "running"},
					Events:     []codersdk.WorkloadEvent{{Stream: stream, Data: []byte("command output")}},
					NextOffset: 1,
				})
			})
			mux.HandleFunc("DELETE /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, r *http.Request) {
				cancellations.Add(1)
				assert.Equal(t, org.String(), r.PathValue("org"))
				assert.Equal(t, id.String(), r.PathValue("id"))
				assert.NoError(t, r.Context().Err())
				select {
				case canceled <- struct{}{}:
				default:
				}
				w.WriteHeader(http.StatusNoContent)
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			writer := &runBlockingWriter{entered: make(chan struct{}, 1), release: release}
			inv := (&serpent.Invocation{Stdout: io.Discard, Stderr: io.Discard}).WithContext(ctx)
			if stream == "stdout" {
				inv.Stdout = writer
			} else {
				// Also cover blocking on diagnostics before streaming begins.
				inv.Stderr = writer
			}
			interrupts := make(chan context.CancelFunc, 1)
			inv = inv.WithTestSignalNotifyContext(t, func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				signalCtx, interrupt := context.WithCancel(parent)
				interrupts <- interrupt
				return signalCtx, interrupt
			})
			root := &RootCmd{clock: quartz.NewMock(t), verbose: stream == "stderr"}
			done := make(chan error, 1)
			go func() {
				done <- root.runWorkload(inv, codersdk.New(serverURL), org, codersdk.CreateWorkloadRequest{ID: id}, nil)
			}()
			interrupt := testutil.TryReceive(ctx, t, interrupts)
			testutil.TryReceive(ctx, t, writer.entered)
			interrupt()
			testutil.TryReceive(ctx, t, canceled)
			require.EqualValues(t, 1, cancellations.Load(), "cancellation must reach the server before output is unblocked")
			// Repeated interrupts must not send duplicate cancellation requests.
			interrupt()
			unblock()
			err = testutil.TryReceive(ctx, t, done)
			var exitErr *exitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, 130, exitErr.ExitCode())
			require.Nil(t, exitErr.Unwrap())
			require.EqualValues(t, 1, cancellations.Load())
		})
	}
}

type runBlockingWriter struct {
	entered chan struct{}
	release <-chan struct{}
}

func (w *runBlockingWriter) Write(data []byte) (int, error) {
	select {
	case w.entered <- struct{}{}:
	default:
	}
	<-w.release
	return len(data), nil
}

func TestRunContinuousStream(t *testing.T) {
	t.Parallel()
	for _, continuousOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("ContinuousOutput=%t", continuousOutput), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := quartz.NewMock(t)
			connected := make(chan struct{}, 1)
			frames := make(chan codersdk.WorkloadOutput)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method, "streaming must not create or cancel workloads")
				assert.Equal(t, "true", r.URL.Query().Get("follow"))
				assert.Equal(t, "0", r.URL.Query().Get("offset"))
				requests.Add(1)
				writeRunTestStream(t, w, codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "running"}})
				connected <- struct{}{}
				for {
					select {
					case <-r.Context().Done():
						return
					case frame := <-frames:
						writeRunTestStream(t, w, frame)
						if frame.Workload.Terminal() {
							return
						}
					}
				}
			}))
			defer server.Close()
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			stdout := &runNotifyingWriter{wrote: make(chan struct{}, 1)}
			var stderr bytes.Buffer
			inv := &serpent.Invocation{Stdout: stdout, Stderr: &stderr}
			done := make(chan error, 1)
			go func() { done <- watchWorkload(ctx, inv, codersdk.New(serverURL), uuid.New(), uuid.New(), clock, nil) }()
			testutil.TryReceive(ctx, t, connected)
			clock.Advance(time.Minute).MustWait(ctx)
			var offset int64
			if continuousOutput {
				for range 600 {
					offset++
					frame := codersdk.WorkloadOutput{
						Workload: codersdk.Workload{State: "running"}, NextOffset: offset,
						Events: []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("x")}},
					}
					testutil.RequireSend(ctx, t, frames, frame)
					if offset == 1 {
						testutil.TryReceive(ctx, t, stdout.wrote)
					}
				}
			}
			testutil.RequireSend(ctx, t, frames, codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "succeeded", ExitCode: new(0)}, NextOffset: offset})
			require.NoError(t, testutil.TryReceive(ctx, t, done))
			require.EqualValues(t, 1, requests.Load(), "all output and idle time must use one HTTP request")
			require.Equal(t, int(offset), stdout.Len())
			require.Empty(t, stderr.String())
		})
	}
}

type runNotifyingWriter struct {
	bytes.Buffer
	wrote chan struct{}
}

func (w *runNotifyingWriter) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	select {
	case w.wrote <- struct{}{}:
	default:
	}
	return n, err
}

func TestRunStreamingRateLimit(t *testing.T) {
	t.Parallel()
	for _, verbose := range []bool{false, true} {
		t.Run(fmt.Sprintf("Verbose=%t", verbose), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := quartz.NewMock(t)
			trap := clock.Trap().NewTimer("workload-reconnect")
			defer trap.Close()
			var polls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method, "throttling must not create or cancel workloads")
				assert.Equal(t, "0", r.URL.Query().Get("offset"))
				switch polls.Add(1) {
				case 1, 2:
					w.Header().Set("Retry-After", "3")
					w.WriteHeader(http.StatusTooManyRequests)
				default:
					writeRunTestStream(t, w, codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "failed", ExitCode: new(23)}})
				}
			}))
			defer server.Close()
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			inv := &serpent.Invocation{Stdout: &stdout, Stderr: &stderr}
			var timings *workloadRunTimings
			if verbose {
				timings = &workloadRunTimings{started: clock.Now()}
			}
			done := make(chan error, 1)
			go func() {
				done <- watchWorkload(ctx, inv, codersdk.New(serverURL), uuid.New(), uuid.New(), clock, timings)
			}()
			for _, delay := range []time.Duration{3 * time.Second, 3 * time.Second} {
				call := trap.MustWait(ctx)
				require.Equal(t, delay, call.Duration)
				call.MustRelease(ctx)
				clock.Advance(delay).MustWait(ctx)
			}
			var exitErr *exitError
			require.ErrorAs(t, testutil.TryReceive(ctx, t, done), &exitErr)
			require.Equal(t, 23, exitErr.ExitCode())
			require.Nil(t, exitErr.Unwrap())
			require.EqualValues(t, 3, polls.Load())
			require.Empty(t, stdout.String())
			if verbose {
				require.Equal(t, 1, strings.Count(stderr.String(), "Output stream rate limited (HTTP 429); reconnecting in 3s."))
				require.NotContains(t, stderr.String(), "Connection interrupted")
			} else {
				require.Empty(t, stderr.String())
			}
		})
	}
}

func TestRunStreamingFailure(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusUnauthorized, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := quartz.NewMock(t)
			trap := clock.Trap().NewTimer("workload-reconnect")
			defer trap.Close()
			template := codersdk.Template{ID: uuid.New(), OrganizationID: uuid.New()}
			var creates, polls, cancels atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v2/templates/{id}", func(w http.ResponseWriter, _ *http.Request) {
				writeRunTestJSON(t, w, template)
			})
			mux.HandleFunc("POST /api/v2/organizations/{org}/workloads", func(w http.ResponseWriter, r *http.Request) {
				creates.Add(1)
				var req codersdk.CreateWorkloadRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				writeRunTestJSON(t, w, codersdk.Workload{ID: req.ID, State: "running"})
			})
			mux.HandleFunc("GET /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, r *http.Request) {
				polls.Add(1)
				assert.Equal(t, "true", r.URL.Query().Get("follow"))
				assert.Equal(t, "0", r.URL.Query().Get("offset"))
				if status == http.StatusOK {
					writeRunTestStream(t, w, codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "running"}})
					return // Repeated empty snapshots cannot reset the retry budget.
				}
				w.WriteHeader(status)
			})
			mux.HandleFunc("DELETE /api/v2/organizations/{org}/workloads/{id}", func(w http.ResponseWriter, _ *http.Request) {
				cancels.Add(1)
				w.WriteHeader(http.StatusNoContent)
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			inv, stdout, _ := newRunTestInvocation(t, server.URL, clock,
				"--template", template.ID.String(), "--cpu", "1", "--ram", "16MiB", "--cmd", "true")
			done := make(chan error, 1)
			go func() { done <- inv.WithContext(ctx).Run() }()
			attempts := 1
			if status != http.StatusUnauthorized {
				attempts = workloadStreamAttempts
			}
			for i := range attempts - 1 {
				call := trap.MustWait(ctx)
				delay := min(50*time.Millisecond*time.Duration(1<<i), time.Second)
				require.Equal(t, delay, call.Duration)
				call.MustRelease(ctx)
				clock.Advance(delay).MustWait(ctx)
			}
			err := testutil.TryReceive(ctx, t, done)
			require.ErrorContains(t, err, "the workload has not been canceled")
			require.EqualValues(t, attempts, polls.Load())
			require.EqualValues(t, 1, creates.Load())
			require.Zero(t, cancels.Load())
			require.Empty(t, stdout.String())
		})
	}
}

func TestRunCreateFailure(t *testing.T) {
	t.Parallel()
	template := codersdk.Template{ID: uuid.New(), OrganizationID: uuid.New()}
	var creates, other atomic.Int32
	ids := make(chan uuid.UUID, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/templates/"+template.ID.String():
			writeRunTestJSON(t, w, template)
		case r.Method == http.MethodPost:
			creates.Add(1)
			var req codersdk.CreateWorkloadRequest
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			select {
			case ids <- req.ID:
			default:
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			other.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	inv, stdout, _ := newRunTestInvocation(t, server.URL, quartz.NewMock(t),
		"--template", template.ID.String(), "--cpu", "1", "--ram", "16MiB", "--cmd", "echo secret-command")
	err := inv.Run()
	require.ErrorContains(t, err, "create workload")
	require.ErrorContains(t, err, testutil.TryReceive(inv.Context(), t, ids).String())
	require.NotContains(t, err.Error(), "secret-command")
	require.EqualValues(t, 1, creates.Load())
	require.Zero(t, other.Load(), "placement failures must not relaunch or cancel the command")
	require.Empty(t, stdout.String())
}

func TestRunOutputEdgeCases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		output     codersdk.WorkloadOutput
		writer     io.Writer
		wantErr    string
		wantStdout string
		verbose    bool
	}{
		{
			name:    "UnknownOutcome",
			output:  codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "unknown"}},
			wantErr: "without an exit code",
		},
		{
			name:    "RejectedCapacity",
			output:  codersdk.WorkloadOutput{Workload: codersdk.Workload{State: "rejected", Failure: "No eligible prepared capacity is available."}},
			wantErr: "No eligible prepared capacity is available.",
		},
		{
			name:    "NoProgress",
			output:  codersdk.WorkloadOutput{Events: []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("duplicate")}}},
			wantErr: "invalid event offset",
		},
		{
			name:    "UnknownStream",
			output:  codersdk.WorkloadOutput{Events: []codersdk.WorkloadEvent{{Stream: "other", Data: []byte("not stdout")}}, NextOffset: 1},
			wantErr: "unknown stream",
		},
		{
			name:    "OutputConsumerDisconnected",
			output:  codersdk.WorkloadOutput{Events: []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("output")}}, NextOffset: 1},
			writer:  runFailingWriter{err: io.ErrClosedPipe},
			wantErr: "closed pipe",
		},
		{
			name:    "ShortWrite",
			output:  codersdk.WorkloadOutput{Events: []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("output")}}, NextOffset: 1},
			writer:  runFailingWriter{},
			wantErr: "short write",
		},
		{
			name:    "TruncatedOutputVerbose",
			verbose: true,
			output: codersdk.WorkloadOutput{
				Workload:   codersdk.Workload{State: "succeeded", ExitCode: new(0)},
				Events:     []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("retained output")}},
				NextOffset: 123, Truncated: true,
			},
			wantStdout: "retained output",
		},
		{
			name: "TruncatedOutput",
			output: codersdk.WorkloadOutput{
				Workload:   codersdk.Workload{State: "succeeded", ExitCode: new(0)},
				Events:     []codersdk.WorkloadEvent{{Stream: "stdout", Data: []byte("retained output")}},
				NextOffset: 123, Truncated: true,
			},
			wantStdout: "retained output",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := quartz.NewMock(t)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method, "observation failures must not cancel or relaunch")
				assert.Equal(t, "true", r.URL.Query().Get("follow"))
				requests.Add(1)
				output := tc.output
				writeRunTestStream(t, w, output)
				if tc.wantErr == "" {
					output.Events = nil
					writeRunTestStream(t, w, output)
				}
			}))
			defer server.Close()
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			inv := &serpent.Invocation{Stdout: stdout, Stderr: stderr}
			if tc.writer != nil {
				inv.Stdout = tc.writer
			}
			var timings *workloadRunTimings
			if tc.verbose {
				timings = &workloadRunTimings{started: clock.Now()}
			}
			done := make(chan error, 1)
			go func() {
				done <- watchWorkload(ctx, inv, codersdk.New(serverURL), uuid.New(), uuid.New(), clock, timings)
			}()
			err = testutil.TryReceive(ctx, t, done)
			if tc.wantErr == "" {
				require.NoError(t, err)
				if tc.verbose {
					require.Contains(t, stderr.String(), "Warning: retained workload output was truncated.\n")
					require.Equal(t, 1, strings.Count(stderr.String(), "Warning:"))
					require.Contains(t, stderr.String(), "Provision      unavailable")
					require.Contains(t, stderr.String(), "Exec           unavailable")
				} else {
					require.Empty(t, stderr.String())
				}
				require.EqualValues(t, 1, requests.Load())
			} else {
				require.ErrorContains(t, err, tc.wantErr)
				require.EqualValues(t, 1, requests.Load())
			}
			require.Equal(t, tc.wantStdout, stdout.String())
		})
	}
}

type runFailingWriter struct {
	err error
}

func (w runFailingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func newRunTestInvocation(t *testing.T, serverURL string, clock quartz.Clock, args ...string) (inv *serpent.Invocation, stdout, stderr *bytes.Buffer) {
	t.Helper()
	root := &RootCmd{clock: clock}
	cmd, err := root.Command(root.CoreSubcommands())
	require.NoError(t, err)
	args = append([]string{"--url", serverURL, "--token", "test-token", "--global-config", t.TempDir(), "run"}, args...)
	inv = cmd.Invoke(args...).WithContext(testutil.Context(t, testutil.WaitLong))
	stdout, stderr = new(bytes.Buffer), new(bytes.Buffer)
	inv.Stdout, inv.Stderr, inv.Stdin = stdout, stderr, strings.NewReader("")
	return inv, stdout, stderr
}

func writeRunTestStream(t *testing.T, w http.ResponseWriter, outputs ...codersdk.WorkloadOutput) {
	t.Helper()
	w.Header().Set("Content-Type", "application/x-ndjson")
	for _, output := range outputs {
		assert.NoError(t, json.NewEncoder(w).Encode(output))
		assert.NoError(t, http.NewResponseController(w).Flush())
	}
}

func writeRunTestJSON(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	assert.NoError(t, json.NewEncoder(w).Encode(data))
}
