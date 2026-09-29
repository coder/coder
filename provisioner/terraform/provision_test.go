//go:build linux || darwin

package terraform_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

type provisionerServeOptions struct {
	binaryPath    string
	cliConfigPath string
	exitTimeout   time.Duration
	workDir       string
	logger        *slog.Logger
}

func setupProvisioner(t *testing.T, opts *provisionerServeOptions) (context.Context, proto.DRPCProvisionerClient) {
	if opts == nil {
		opts = &provisionerServeOptions{}
	}
	cachePath := t.TempDir()
	if opts.workDir == "" {
		opts.workDir = t.TempDir()
	}
	if opts.logger == nil {
		logger := testutil.Logger(t)
		opts.logger = &logger
	}
	client, server := drpcsdk.MemTransportPipe()
	ctx, cancelFunc := context.WithCancel(context.Background())
	serverErr := make(chan error, 1)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		cancelFunc()
		err := <-serverErr
		if !errors.Is(err, context.Canceled) {
			assert.NoError(t, err)
		}
	})
	go func() {
		serverErr <- terraform.Serve(ctx, &terraform.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Listener:      server,
				Logger:        *opts.logger,
				WorkDirectory: opts.workDir,
			},
			BinaryPath:    opts.binaryPath,
			CachePath:     cachePath,
			ExitTimeout:   opts.exitTimeout,
			CliConfigPath: opts.cliConfigPath,
		})
	}()
	api := proto.NewDRPCProvisionerClient(client)

	return ctx, api
}

// sendInitAndGetResp will send the init request and wait for and return the InitComplete response.
func sendInitAndGetResp(t *testing.T, sess proto.DRPCProvisioner_SessionClient, archive []byte, onLog ...func(log string)) *proto.InitComplete {
	t.Helper()
	err := sendInit(sess, archive)
	require.NoError(t, err)
	for {
		msg, err := sess.Recv()
		require.NoError(t, err)
		if logMsg, ok := msg.Type.(*proto.Response_Log); ok {
			for _, do := range onLog {
				do(logMsg.Log.Output)
			}
			continue
		}

		init := msg.GetInit()
		require.NotNil(t, init)
		return init
	}
}

func configure(ctx context.Context, t *testing.T, client proto.DRPCProvisionerClient, config *proto.Config) proto.DRPCProvisioner_SessionClient {
	t.Helper()
	sess, err := client.Session(ctx)
	require.NoError(t, err)
	err = sess.Send(&proto.Request{Type: &proto.Request_Config{Config: config}})
	require.NoError(t, err)
	return sess
}

func readProvisionLog(t *testing.T, response proto.DRPCProvisioner_SessionClient) (string, *proto.Response) {
	var last *proto.Response
	var logBuf strings.Builder
	for {
		msg, err := response.Recv()
		require.NoError(t, err)

		if log := msg.GetLog(); log != nil {
			t.Log(log.Level.String(), log.Output)
			_, err = logBuf.WriteString(log.Output)
			require.NoError(t, err)
			continue
		}
		last = msg
		break
	}
	return logBuf.String(), last
}

func sendInit(sess proto.DRPCProvisioner_SessionClient, archive []byte) error {
	return sess.Send(&proto.Request{Type: &proto.Request_Init{Init: &proto.InitRequest{
		TemplateSourceArchive: archive,
	}}})
}

func sendPlan(sess proto.DRPCProvisioner_SessionClient, transition proto.WorkspaceTransition) error {
	return sess.Send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
		Metadata: &proto.Metadata{WorkspaceTransition: transition},
	}}})
}

func sendApply(sess proto.DRPCProvisioner_SessionClient, transition proto.WorkspaceTransition) error {
	return sess.Send(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{
		Metadata: &proto.Metadata{WorkspaceTransition: transition},
	}}})
}

func sendGraph(sess proto.DRPCProvisioner_SessionClient, source proto.GraphSource) error {
	return sess.Send(&proto.Request{Type: &proto.Request_Graph{Graph: &proto.GraphRequest{
		Source: source,
	}}})
}

func TestGraphCapturesSavedPlanGraphWhenRequired(t *testing.T) {
	t.Parallel()

	const selectedPlanJSON = `{
		"format_version":"1.2",
		"terraform_version":"1.15.0",
		"planned_values":{"root_module":{"resources":[
			{"address":"docker_container.workspace","mode":"managed","type":"docker_container","name":"workspace","values":{}},
			{"address":"coder_agent.main","mode":"managed","type":"coder_agent","name":"main","values":{"arch":"amd64","id":"agent-id"}},
			{"address":"coder_script.prerequisite","mode":"managed","type":"coder_script","name":"prerequisite","values":{"agent_id":"agent-id","run_on_start":true}},
			{"address":"coder_script.dependent","mode":"managed","type":"coder_script","name":"dependent","values":{"agent_id":"agent-id","run_on_start":true}},
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","values":{"rule":[{"run":["coder_script.dependent"],"after":["coder_script.prerequisite"]}]}}
		]}},
		"configuration":{"root_module":{"resources":[
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","schema_version":0}
		]}}
	}`
	const shutdownPlanJSON = `{
		"format_version":"1.2",
		"terraform_version":"1.15.0",
		"planned_values":{"root_module":{"resources":[
			{"address":"docker_container.workspace","mode":"managed","type":"docker_container","name":"workspace","values":{}},
			{"address":"coder_agent.main","mode":"managed","type":"coder_agent","name":"main","values":{"arch":"amd64","id":"agent-id"}},
			{"address":"coder_script.prerequisite","mode":"managed","type":"coder_script","name":"prerequisite","values":{"agent_id":"agent-id","run_on_stop":true}},
			{"address":"coder_script.dependent","mode":"managed","type":"coder_script","name":"dependent","values":{"agent_id":"agent-id","run_on_stop":true}},
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","values":{"rule":[{"run":["coder_script.dependent"],"after":["coder_script.prerequisite"],"phase":"stop"}]}}
		]}},
		"configuration":{"root_module":{"resources":[
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","schema_version":0}
		]}}
	}`
	const emptySelectionPlanJSON = `{
		"format_version":"1.2",
		"terraform_version":"1.15.0",
		"planned_values":{"root_module":{"resources":[
			{"address":"docker_container.workspace","mode":"managed","type":"docker_container","name":"workspace","values":{}},
			{"address":"coder_agent.main","mode":"managed","type":"coder_agent","name":"main","values":{"arch":"amd64","id":"agent-id"}},
			{"address":"coder_script.prerequisite","mode":"managed","type":"coder_script","name":"prerequisite","values":{"agent_id":"agent-id","run_on_start":true}},
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","values":{"rule":[{"run":["coder_script.optional"],"after":["coder_script.prerequisite"]}]}}
		]}},
		"configuration":{"root_module":{"resources":[
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","schema_version":0},
			{"address":"coder_script.optional","mode":"managed","type":"coder_script","name":"optional","schema_version":0,"count_expression":{"constant_value":0}}
		]}}
	}`
	const noDeclarationPlanJSON = `{
		"format_version":"1.2",
		"terraform_version":"1.15.0",
		"planned_values":{"root_module":{"resources":[
			{"address":"coder_agent.main","mode":"managed","type":"coder_agent","name":"main","values":{"arch":"amd64","id":"agent-id"}}
		]}},
		"configuration":{"root_module":{"resources":[
			{"address":"coder_agent.main","mode":"managed","type":"coder_agent","name":"main","schema_version":0}
		]}}
	}`
	const removedDeclarationPlanJSON = `{
		"format_version":"1.2",
		"terraform_version":"1.15.0",
		"planned_values":{"root_module":{}},
		"prior_state":{
			"format_version":"1.0",
			"terraform_version":"1.15.0",
			"values":{"root_module":{"resources":[
				{"address":"data.coder_script_order.order[0]","mode":"data","type":"coder_script_order","name":"order","index":0,"values":{"rule":[{"run":["coder_script.missing"],"after":["coder_script.also_missing"]}]}}
			]}}
		},
		"configuration":{"root_module":{"resources":[
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","schema_version":0,"count_expression":{"constant_value":0}}
		]}}
	}`
	const phaseWarningPlanJSON = `{
		"format_version":"1.2",
		"terraform_version":"1.15.0",
		"planned_values":{"root_module":{
			"resources":[
				{"address":"docker_container.workspace","mode":"managed","type":"docker_container","name":"workspace","values":{}},
				{"address":"coder_agent.main","mode":"managed","type":"coder_agent","name":"main","values":{"arch":"amd64","id":"agent-id"}},
				{"address":"coder_script.dependent","mode":"managed","type":"coder_script","name":"dependent","values":{"agent_id":"agent-id","run_on_start":true}},
				{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","values":{"rule":[{"run":["coder_script.dependent"],"after":["module.bootstrap"]}]}}
			],
			"child_modules":[{
				"address":"module.bootstrap",
				"resources":[
					{"address":"module.bootstrap.coder_script.start","mode":"managed","type":"coder_script","name":"start","values":{"agent_id":"agent-id","run_on_start":true}},
					{"address":"module.bootstrap.coder_script.stop","mode":"managed","type":"coder_script","name":"stop","values":{"agent_id":"agent-id","run_on_stop":true}}
				]
			}]
		}},
		"configuration":{"root_module":{
			"resources":[
				{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","schema_version":0}
			],
			"module_calls":{
				"bootstrap":{"source":"./bootstrap","module":{"resources":[]}}
			}
		}}
	}`
	const graphDOT = `digraph {
		"[root] docker_container.workspace" [label = "docker_container.workspace"]
		"[root] coder_agent.main (expand)" [label = "coder_agent.main"]
		"[root] docker_container.workspace" -> "[root] coder_agent.main (expand)"
	}`

	tests := []struct {
		name             string
		transition       proto.WorkspaceTransition
		planJSON         string
		savedPlanGraph   string
		wantGraphs       int
		wantAgentScripts int
		errorContains    string
		warningContains  string
	}{
		{
			name:       "StartSelectedScripts",
			transition: proto.WorkspaceTransition_START,
			planJSON:   selectedPlanJSON,
			wantGraphs: 2,
		},
		{
			name:       "StartEmptySelection",
			transition: proto.WorkspaceTransition_START,
			planJSON:   emptySelectionPlanJSON,
			wantGraphs: 1,
		},
		{
			name:       "StartWithoutDeclaration",
			transition: proto.WorkspaceTransition_START,
			planJSON:   noDeclarationPlanJSON,
			wantGraphs: 1,
		},
		{
			name:             "Stop",
			transition:       proto.WorkspaceTransition_STOP,
			planJSON:         shutdownPlanJSON,
			wantGraphs:       1,
			wantAgentScripts: 2,
		},
		{
			name:             "Destroy",
			transition:       proto.WorkspaceTransition_DESTROY,
			planJSON:         shutdownPlanJSON,
			wantGraphs:       1,
			wantAgentScripts: 2,
		},
		{
			name:           "MalformedSavedPlanGraph",
			transition:     proto.WorkspaceTransition_START,
			planJSON:       selectedPlanJSON,
			savedPlanGraph: "not a graph",
			wantGraphs:     2,
			errorContains:  "index saved plan graph for script ordering",
		},
		{
			name:           "RemovedDeclaration",
			transition:     proto.WorkspaceTransition_START,
			planJSON:       removedDeclarationPlanJSON,
			savedPlanGraph: `digraph { "[root] data.coder_script_order.order[0] (destroy)" }`,
			wantGraphs:     2,
		},
		{
			name:            "StartPhaseWarning",
			transition:      proto.WorkspaceTransition_START,
			planJSON:        phaseWarningPlanJSON,
			wantGraphs:      2,
			warningContains: `inferred phase "start"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			planPath := filepath.Join(dir, "plan.json")
			graphPath := filepath.Join(dir, "graph.dot")
			savedPlanGraphPath := filepath.Join(dir, "saved-plan-graph.dot")
			graphArgsPath := filepath.Join(dir, "graph-args")
			require.NoError(t, os.WriteFile(planPath, []byte(test.planJSON), 0o600))
			require.NoError(t, os.WriteFile(graphPath, []byte(graphDOT), 0o600))
			savedPlanGraph := test.savedPlanGraph
			if savedPlanGraph == "" {
				savedPlanGraph = graphDOT
			}
			require.NoError(t, os.WriteFile(
				savedPlanGraphPath, []byte(savedPlanGraph), 0o600,
			))

			binaryPath := filepath.Join(dir, "terraform")
			binary := fmt.Sprintf(`#!/bin/sh
case "$1" in
version)
	printf '%%s\n' '{"terraform_version":"1.15.0","platform":"linux_amd64","provider_selections":{}}'
	;;
show)
	cat %q
	;;
graph)
	printf '%%s\n' "$*" >> %q
	case "$*" in
	*-plan=*) cat %q ;;
	*) cat %q ;;
	esac
	;;
init)
	exit 0
	;;
*)
	exit 1
	;;
esac
`, planPath, graphArgsPath, savedPlanGraphPath, graphPath)
			require.NoError(t, os.WriteFile(binaryPath, []byte(binary), 0o700))

			ctx, api := setupProvisioner(t, &provisionerServeOptions{
				binaryPath: binaryPath,
			})
			sess := configure(ctx, t, api, &proto.Config{})
			init := sendInitAndGetResp(t, sess, testutil.CreateTar(t, nil))
			require.Empty(t, init.Error)
			require.NoError(t, sess.Send(&proto.Request{
				Type: &proto.Request_Graph{Graph: &proto.GraphRequest{
					Metadata: &proto.Metadata{WorkspaceTransition: test.transition},
					Source:   proto.GraphSource_SOURCE_PLAN,
				}},
			}))
			logs, response := readProvisionLog(t, sess)
			graph := response.GetGraph()
			require.NotNil(t, graph)
			if test.warningContains != "" {
				require.Contains(t, logs, test.warningContains)
				require.Contains(t, logs, "module.bootstrap")
			}
			if test.errorContains == "" {
				require.Empty(t, graph.Error)
			} else {
				require.Contains(t, graph.Error, test.errorContains)
			}
			if test.wantAgentScripts > 0 {
				var scripts []*proto.Script
				for _, resource := range graph.Resources {
					for _, agent := range resource.Agents {
						scripts = append(scripts, agent.Scripts...)
					}
				}
				require.Len(t, scripts, test.wantAgentScripts)
			}

			graphArgs, err := os.ReadFile(graphArgsPath)
			require.NoError(t, err)
			commands := strings.Split(strings.TrimSpace(string(graphArgs)), "\n")
			require.Len(t, commands, test.wantGraphs)
			require.Equal(t, "graph -type=plan", commands[0])
			if test.wantGraphs == 2 {
				savedPlanPath, ok := strings.CutPrefix(
					commands[1], "graph -plan=",
				)
				require.True(t, ok)
				require.Equal(t, "terraform.tfplan", filepath.Base(savedPlanPath))
			}
		})
	}
}

func TestGraphReusesCachedScriptOrderPlanConfiguration(t *testing.T) {
	t.Parallel()

	const planJSON = `{
		"format_version":"1.2",
		"terraform_version":"1.15.0",
		"planned_values":{"root_module":{"resources":[
			{"address":"docker_container.workspace","mode":"managed","type":"docker_container","name":"workspace","values":{}},
			{"address":"coder_agent.main","mode":"managed","type":"coder_agent","name":"main","values":{"arch":"amd64","id":"agent-id"}},
			{"address":"coder_script.prerequisite","mode":"managed","type":"coder_script","name":"prerequisite","values":{"agent_id":"agent-id","run_on_start":true}},
			{"address":"coder_script.dependent","mode":"managed","type":"coder_script","name":"dependent","values":{"agent_id":"agent-id","run_on_start":true}},
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","values":{"rule":[{"run":["coder_script.dependent"],"after":["coder_script.prerequisite"]}]}}
		]}},
		"configuration":{"root_module":{"resources":[
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","schema_version":0}
		]}}
	}`
	const stateJSON = `{
		"format_version":"1.0",
		"terraform_version":"1.15.0",
		"values":{"root_module":{"resources":[
			{"address":"docker_container.workspace","mode":"managed","type":"docker_container","name":"workspace","values":{}},
			{"address":"coder_agent.main","mode":"managed","type":"coder_agent","name":"main","values":{"arch":"amd64","id":"agent-id"}},
			{"address":"coder_script.prerequisite","mode":"managed","type":"coder_script","name":"prerequisite","values":{"agent_id":"agent-id","run_on_start":true}},
			{"address":"coder_script.dependent","mode":"managed","type":"coder_script","name":"dependent","values":{"agent_id":"agent-id","run_on_start":true}},
			{"address":"data.coder_script_order.order","mode":"data","type":"coder_script_order","name":"order","values":{"rule":[{"run":["coder_script.dependent"],"after":["coder_script.prerequisite"]}]}}
		]}}
	}`
	const graphDOT = `digraph {
		"[root] docker_container.workspace" [label = "docker_container.workspace"]
		"[root] coder_agent.main (expand)" [label = "coder_agent.main"]
		"[root] docker_container.workspace" -> "[root] coder_agent.main (expand)"
	}`

	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	statePath := filepath.Join(dir, "state.json")
	graphPath := filepath.Join(dir, "graph.dot")
	showArgsPath := filepath.Join(dir, "show-args")
	require.NoError(t, os.WriteFile(planPath, []byte(planJSON), 0o600))
	require.NoError(t, os.WriteFile(statePath, []byte(stateJSON), 0o600))
	require.NoError(t, os.WriteFile(graphPath, []byte(graphDOT), 0o600))

	binaryPath := filepath.Join(dir, "terraform")
	binary := fmt.Sprintf(`#!/bin/sh
case "$1" in
version)
	printf '%%s\n' '{"terraform_version":"1.15.0","platform":"linux_amd64","provider_selections":{}}'
	;;
show)
	printf '%%s\n' "$*" >> %q
	if [ "$#" -eq 3 ]; then
		cat %q
	else
		cat %q
	fi
	;;
graph)
	cat %q
	;;
plan|init)
	exit 0
	;;
*)
	exit 1
	;;
esac
`, showArgsPath, statePath, planPath, graphPath)
	require.NoError(t, os.WriteFile(binaryPath, []byte(binary), 0o700))

	ctx, api := setupProvisioner(t, &provisionerServeOptions{
		binaryPath: binaryPath,
	})
	sess := configure(ctx, t, api, &proto.Config{})
	init := sendInitAndGetResp(t, sess, testutil.CreateTar(t, nil))
	require.Empty(t, init.Error)
	require.NoError(t, sendPlan(sess, proto.WorkspaceTransition_START))
	_, response := readProvisionLog(t, sess)
	plan := response.GetPlan()
	require.NotNil(t, plan)
	require.Empty(t, plan.Error)
	require.NoError(t, sess.Send(&proto.Request{
		Type: &proto.Request_Graph{Graph: &proto.GraphRequest{
			Metadata: &proto.Metadata{
				WorkspaceTransition: proto.WorkspaceTransition_START,
			},
			Source: proto.GraphSource_SOURCE_STATE,
		}},
	}))
	_, response = readProvisionLog(t, sess)
	graph := response.GetGraph()
	require.NotNil(t, graph)
	require.Empty(t, graph.Error)

	showArgs, err := os.ReadFile(showArgsPath)
	require.NoError(t, err)
	commands := strings.Split(strings.TrimSpace(string(showArgs)), "\n")
	require.Len(t, commands, 2)
	require.Contains(t, commands[0], "terraform.tfplan")
	require.Equal(t, "show -json -no-color", commands[1])
}

// below we exec fake_cancel.sh, which causes the kernel to execute it, and if more than
// one process tries to do this simultaneously, it can cause "text file busy"
// nolint: paralleltest
func TestProvision_Cancel(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	fakeBin := filepath.Join(cwd, "testdata", "fake_cancel.sh")

	tests := []struct {
		name          string
		mode          string
		startSequence []string
		wantLog       []string
	}{
		{
			name:          "Cancel init",
			mode:          "init",
			startSequence: []string{"init_start"},
			wantLog:       []string{"interrupt", "exit"},
		},
		{
			// Provisioner requires a plan before an apply, so test cancel with plan.
			name:          "Cancel plan",
			mode:          "plan",
			startSequence: []string{"init", "plan_start"},
			wantLog:       []string{"interrupt", "exit"},
		},
	}
	for _, tt := range tests {
		// below we exec fake_cancel.sh, which causes the kernel to execute it, and if more than
		// one process tries to do this, it can cause "text file busy"
		// nolint: paralleltest
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			binPath := filepath.Join(dir, "terraform")

			// Example: exec /path/to/terrafork_fake_cancel.sh 1.2.1 apply "$@"
			content := fmt.Sprintf("#!/usr/bin/env sh\nexec %q %s %s \"$@\"\n", fakeBin, terraform.TerraformVersion.String(), tt.mode)
			err := os.WriteFile(binPath, []byte(content), 0o755) //#nosec
			require.NoError(t, err)
			t.Logf("wrote fake terraform script to %s", binPath)

			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
				With(slog.F("source", "provisioner")).
				Leveled(slog.LevelDebug)

			ctx, api := setupProvisioner(t, &provisionerServeOptions{
				binaryPath: binPath,
				logger:     &logger,
			})
			sess := configure(ctx, t, api, &proto.Config{})

			err = sendInit(sess, testutil.CreateTar(t, nil))
			require.NoError(t, err)

			var planOnce sync.Once

			for _, line := range tt.startSequence {
			LoopStart:
				msg, err := sess.Recv()
				require.NoError(t, err)

				t.Log(msg.Type)
				if msg.GetInit() != nil && msg.GetInit().GetError() == "" {
					planOnce.Do(func() {
						t.Log("Sending terraform plan request")
						// Send plan after init
						err = sendPlan(sess, proto.WorkspaceTransition_START)
						require.NoError(t, err)
					})
					goto LoopStart
				}

				log := msg.GetLog()
				if log == nil {
					goto LoopStart
				}

				require.Equal(t, line, log.Output)
			}

			t.Log("Sending the cancel request")
			err = sess.Send(&proto.Request{
				Type: &proto.Request_Cancel{
					Cancel: &proto.CancelRequest{},
				},
			})
			require.NoError(t, err)

			var gotLog []string
			for {
				msg, err := sess.Recv()
				require.NoError(t, err)

				if log := msg.GetLog(); log != nil {
					gotLog = append(gotLog, log.Output)
				} else if c := msg.GetPlan(); c != nil {
					require.Contains(t, c.Error, "exit status 1")
					break
				} else if c := msg.GetInit(); c != nil {
					require.Contains(t, c.Error, "exit status 1")
					break
				} else {
					t.Fatalf("unexpected message: %v", msg)
				}
			}
			require.Equal(t, tt.wantLog, gotLog)
		})
	}
}

// below we exec fake_cancel_hang.sh, which causes the kernel to execute it, and if more than
// one process tries to do this, it can cause "text file busy"
// nolint: paralleltest
func TestProvision_CancelTimeout(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	fakeBin := filepath.Join(cwd, "testdata", "fake_cancel_hang.sh")

	dir := t.TempDir()
	binPath := filepath.Join(dir, "terraform")

	// Example: exec /path/to/terraform_fake_cancel.sh 1.2.1 apply "$@"
	content := fmt.Sprintf("#!/bin/sh\nexec %q %s \"$@\"\n", fakeBin, terraform.TerraformVersion.String())
	err = os.WriteFile(binPath, []byte(content), 0o755) //#nosec
	require.NoError(t, err)

	ctx, api := setupProvisioner(t, &provisionerServeOptions{
		binaryPath:  binPath,
		exitTimeout: time.Second,
	})

	sess := configure(ctx, t, api, &proto.Config{})
	sendInitAndGetResp(t, sess, testutil.CreateTar(t, nil))

	// provisioner requires plan before apply, so test cancel with plan.
	err = sendPlan(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	for _, line := range []string{"plan_start"} {
	LoopStart:
		msg, err := sess.Recv()
		require.NoError(t, err)

		t.Log(msg.Type)

		log := msg.GetLog()
		if log == nil {
			goto LoopStart
		}
		require.Equal(t, line, log.Output)
	}

	err = sess.Send(&proto.Request{Type: &proto.Request_Cancel{Cancel: &proto.CancelRequest{}}})
	require.NoError(t, err)

	for {
		msg, err := sess.Recv()
		require.NoError(t, err)

		if c := msg.GetPlan(); c != nil {
			require.Contains(t, c.Error, "killed")
			break
		}
	}
}

// below we exec fake_text_file_busy.sh, which causes the kernel to execute it, and if more than
// one process tries to do this, it can cause "text file busy" to be returned to us. In this test
// we want to simulate "text file busy" getting logged by terraform, due to an issue with the
// terraform-provider-coder
// nolint: paralleltest
func TestProvision_TextFileBusy(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	fakeBin := filepath.Join(cwd, "testdata", "fake_text_file_busy.sh")

	dir := t.TempDir()
	binPath := filepath.Join(dir, "terraform")

	// Example: exec /path/to/terraform_fake_cancel.sh 1.2.1 apply "$@"
	content := fmt.Sprintf("#!/bin/sh\nexec %q %s \"$@\"\n", fakeBin, terraform.TerraformVersion.String())
	err = os.WriteFile(binPath, []byte(content), 0o755) //#nosec
	require.NoError(t, err)

	workDir := t.TempDir()

	err = os.Mkdir(filepath.Join(workDir, ".coder"), 0o700)
	require.NoError(t, err)
	l, err := net.Listen("unix", filepath.Join(workDir, ".coder", "pprof"))
	require.NoError(t, err)
	defer l.Close()
	handlerCalled := 0
	// nolint: gosec
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/debug/pprof/goroutine", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("thestacks\n"))
			assert.NoError(t, err)
			handlerCalled++
		}),
	}
	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.Serve(l)
	}()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	ctx, api := setupProvisioner(t, &provisionerServeOptions{
		binaryPath:  binPath,
		exitTimeout: time.Second,
		workDir:     workDir,
		logger:      &logger,
	})

	sess := configure(ctx, t, api, &proto.Config{})

	err = sendInit(sess, testutil.CreateTar(t, nil))
	require.NoError(t, err)

	found := false
	for {
		msg, err := sess.Recv()
		require.NoError(t, err)

		if c := msg.GetInit(); c != nil {
			require.Contains(t, c.Error, "exit status 1")
			found = true
			break
		}
	}
	require.True(t, found)
	require.EqualValues(t, 1, handlerCalled)
}

func TestProvision(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name     string
		Files    map[string]string
		Metadata *proto.Metadata
		Request  *proto.PlanRequest
		// Response may be nil to not check the response.
		Response              *proto.GraphComplete
		InitResponse          *proto.InitComplete
		InitErrorContains     string
		InitExpectLogContains string
		// If ErrorContains is not empty, PlanComplete should have an Error containing the given string
		PlanErrorContains string
		// If PlanExpectLogContains is not empty, then the logs should contain it.
		PlanExpectLogContains string
		// If Apply is true, then send an Apply request and check we get the same Resources as in Response.
		Apply bool
		// Some tests may need to be skipped until the relevant provider version is released.
		SkipReason string
		// If SkipCacheProviders is true, then skip caching the terraform providers for this test.
		SkipCacheProviders bool
	}{
		{
			Name: "missing-variable",
			Files: map[string]string{
				"main.tf": `variable "A" {
			}`,
			},
			PlanErrorContains:     "terraform plan:",
			PlanExpectLogContains: "No value for required variable",
		},
		{
			Name: "missing-variable-dry-run",
			Files: map[string]string{
				"main.tf": `variable "A" {
			}`,
			},
			PlanErrorContains:     "terraform plan:",
			PlanExpectLogContains: "No value for required variable",
		},
		{
			Name: "single-resource-dry-run",
			Files: map[string]string{
				"main.tf": `resource "null_resource" "A" {}`,
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "A",
					Type: "null_resource",
				}},
			},
		},
		{
			Name: "single-resource",
			Files: map[string]string{
				"main.tf": `resource "null_resource" "A" {}`,
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "A",
					Type: "null_resource",
				}},
			},
			Apply: true,
		},
		{
			Name: "single-resource-json",
			Files: map[string]string{
				"main.tf.json": `{
					"resource": {
						"null_resource": {
							"A": [
								{}
							]
						}
					}
				}`,
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "A",
					Type: "null_resource",
				}},
			},
			Apply: true,
		},
		{
			Name: "bad-syntax-1",
			Files: map[string]string{
				"main.tf": `a`,
			},
			InitErrorContains:     "initialize terraform",
			InitExpectLogContains: "Argument or block definition required",
			SkipCacheProviders:    true,
		},
		{
			Name: "bad-syntax-2",
			Files: map[string]string{
				"main.tf": `;asdf;`,
			},
			InitErrorContains:     "initialize terraform",
			InitExpectLogContains: `The ";" character is not valid.`,
			SkipCacheProviders:    true,
		},
		{
			Name: "destroy-no-state",
			Files: map[string]string{
				"main.tf": `resource "null_resource" "A" {}`,
			},
			Metadata: &proto.Metadata{
				WorkspaceTransition: proto.WorkspaceTransition_DESTROY,
			},
			PlanExpectLogContains: "nothing to do",
		},
		{
			Name: "rich-parameter-with-value",
			Files: map[string]string{
				"main.tf": `terraform {
					required_providers {
					  coder = {
						source  = "coder/coder"
						version = "0.6.20"
					  }
					}
				  }

				  data "coder_parameter" "sample" {
					name = "Sample"
					type = "string"
					default = "foobaz"
				  }

				  data "coder_parameter" "example" {
					name = "Example"
					type = "string"
					default = "foobar"
				  }

				  resource "null_resource" "example" {
					triggers = {
						misc = "${data.coder_parameter.example.value}"
					}
				  }`,
			},
			Request: &proto.PlanRequest{
				RichParameterValues: []*proto.RichParameterValue{
					{
						Name:  "Example",
						Value: "foobaz",
					},
					{
						Name:  "Sample",
						Value: "foofoo",
					},
				},
			},
			Response: &proto.GraphComplete{
				Parameters: []*proto.RichParameter{
					{
						Name:         "Example",
						Type:         "string",
						DefaultValue: "foobar",
					},
					{
						Name:         "Sample",
						Type:         "string",
						DefaultValue: "foobaz",
					},
				},
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "null_resource",
				}},
			},
		},
		{
			Name: "rich-parameter-with-value-json",
			Files: map[string]string{
				"main.tf.json": `{
					"data": {
						"coder_parameter": {
							"example": [
								{
									"default": "foobar",
									"name": "Example",
									"type": "string"
								}
							],
							"sample": [
								{
									"default": "foobaz",
									"name": "Sample",
									"type": "string"
								}
							]
						}
					},
					"resource": {
						"null_resource": {
							"example": [
								{
									"triggers": {
										"misc": "${data.coder_parameter.example.value}"
									}
								}
							]
						}
					},
					"terraform": [
						{
							"required_providers": [
								{
									"coder": {
										"source": "coder/coder",
										"version": "0.6.20"
									}
								}
							]
						}
					]
				}`,
			},
			Request: &proto.PlanRequest{
				RichParameterValues: []*proto.RichParameterValue{
					{
						Name:  "Example",
						Value: "foobaz",
					},
					{
						Name:  "Sample",
						Value: "foofoo",
					},
				},
			},
			Response: &proto.GraphComplete{
				Parameters: []*proto.RichParameter{
					{
						Name:         "Example",
						Type:         "string",
						DefaultValue: "foobar",
					},
					{
						Name:         "Sample",
						Type:         "string",
						DefaultValue: "foobaz",
					},
				},
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "null_resource",
				}},
			},
		},
		{
			Name: "git-auth",
			Files: map[string]string{
				"main.tf": `terraform {
					required_providers {
					  coder = {
						source  = "coder/coder"
						version = "0.6.20"
					  }
					}
				}

				data "coder_git_auth" "github" {
					id = "github"
				}

				resource "null_resource" "example" {}

				resource "coder_metadata" "example" {
					resource_id = null_resource.example.id
					item {
						key = "token"
						value = data.coder_git_auth.github.access_token
					}
				}
				`,
			},
			Request: &proto.PlanRequest{
				ExternalAuthProviders: []*proto.ExternalAuthProvider{{
					Id:          "github",
					AccessToken: "some-value",
				}},
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "null_resource",
					Metadata: []*proto.Resource_Metadata{{
						Key:   "token",
						Value: "some-value",
					}},
				}},
			},
		},
		{
			Name: "ssh-key",
			Files: map[string]string{
				"main.tf": `terraform {
					required_providers {
					  coder = {
						source  = "coder/coder"
					  }
					}
				}

				resource "null_resource" "example" {}
				data "coder_workspace_owner" "me" {}
				resource "coder_metadata" "example" {
					resource_id = null_resource.example.id
					item {
						key = "pubkey"
						value = data.coder_workspace_owner.me.ssh_public_key
					}
					item {
						key = "privkey"
						value = data.coder_workspace_owner.me.ssh_private_key
					}
				}
				`,
			},
			Request: &proto.PlanRequest{
				Metadata: &proto.Metadata{
					WorkspaceOwnerSshPublicKey:  "fake public key",
					WorkspaceOwnerSshPrivateKey: "fake private key",
				},
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "null_resource",
					Metadata: []*proto.Resource_Metadata{{
						Key:   "pubkey",
						Value: "fake public key",
					}, {
						Key:   "privkey",
						Value: "fake private key",
					}},
				}},
			},
		},
		{
			Name:       "workspace-owner-login-type",
			SkipReason: "field will be added in provider version 1.1.0",
			Files: map[string]string{
				"main.tf": `terraform {
					required_providers {
					  coder = {
						source  = "coder/coder"
						version = "1.1.0"
					  }
					}
				}

				resource "null_resource" "example" {}
				data "coder_workspace_owner" "me" {}
				resource "coder_metadata" "example" {
					resource_id = null_resource.example.id
					item {
						key = "login_type"
						value = data.coder_workspace_owner.me.login_type
					}
				}
				`,
			},
			Request: &proto.PlanRequest{
				Metadata: &proto.Metadata{
					WorkspaceOwnerLoginType: "github",
				},
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "null_resource",
					Metadata: []*proto.Resource_Metadata{{
						Key:   "login_type",
						Value: "github",
					}},
				}},
			},
		},
		{
			Name: "returns-modules",
			Files: map[string]string{
				"main.tf": `module "hello" {
                    source = "./module"
                  }`,
				"module/module.tf": `
				  resource "null_resource" "example" {}

				  module "there" {
					source = "./inner_module"
				  }
				`,
				"module/inner_module/inner_module.tf": `
				  resource "null_resource" "inner_example" {}
				`,
			},
			Request: &proto.PlanRequest{},
			InitResponse: &proto.InitComplete{
				Modules: []*proto.Module{{
					Key:     "hello",
					Version: "",
					Source:  "./module",
				}, {
					Key:     "hello.there",
					Version: "",
					Source:  "./inner_module",
				}},
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name:       "example",
					Type:       "null_resource",
					ModulePath: "module.hello",
				}, {
					Name:       "inner_example",
					Type:       "null_resource",
					ModulePath: "module.hello.module.there",
				}},
			},
		},
		{
			Name:       "workspace-owner-rbac-roles",
			SkipReason: "field will be added in provider version 2.2.0",
			Files: map[string]string{
				"main.tf": `terraform {
					required_providers {
					  coder = {
						source  = "coder/coder"
						version = "2.2.0"
					  }
					}
				}

				resource "null_resource" "example" {}
				data "coder_workspace_owner" "me" {}
				resource "coder_metadata" "example" {
					resource_id = null_resource.example.id
					item {
						key = "rbac_roles_name"
						value = data.coder_workspace_owner.me.rbac_roles[0].name
					}
					item {
						key = "rbac_roles_org_id"
						value = data.coder_workspace_owner.me.rbac_roles[0].org_id
					}
				}
				`,
			},
			Request: &proto.PlanRequest{
				Metadata: &proto.Metadata{
					WorkspaceOwnerRbacRoles: []*proto.Role{{Name: "member", OrgId: ""}},
				},
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "null_resource",
					Metadata: []*proto.Resource_Metadata{{
						Key:   "rbac_roles_name",
						Value: "member",
					}, {
						Key:   "rbac_roles_org_id",
						Value: "",
					}},
				}},
			},
		},
		{
			Name: "is-prebuild",
			Files: map[string]string{
				"main.tf": `terraform {
					required_providers {
					  coder = {
						source  = "coder/coder"
						version = ">= 2.4.1"
					  }
					}
				}
				data "coder_workspace" "me" {}
				resource "null_resource" "example" {}
				resource "coder_metadata" "example" {
					resource_id = null_resource.example.id
					item {
						key = "is_prebuild"
						value = data.coder_workspace.me.is_prebuild
					}
				}
				`,
			},
			Request: &proto.PlanRequest{
				Metadata: &proto.Metadata{
					PrebuiltWorkspaceBuildStage: proto.PrebuiltWorkspaceBuildStage_CREATE,
				},
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "null_resource",
					Metadata: []*proto.Resource_Metadata{{
						Key:   "is_prebuild",
						Value: "true",
					}},
				}},
			},
		},
		{
			Name: "is-prebuild-claim",
			Files: map[string]string{
				"main.tf": `terraform {
					required_providers {
					  coder = {
						source  = "coder/coder"
						version = ">= 2.4.1"
					  }
					}
				}
				data "coder_workspace" "me" {}
				resource "null_resource" "example" {}
				resource "coder_metadata" "example" {
					resource_id = null_resource.example.id
					item {
						key = "is_prebuild_claim"
						value = data.coder_workspace.me.is_prebuild_claim
					}
				}
				`,
			},
			Request: &proto.PlanRequest{
				Metadata: &proto.Metadata{
					PrebuiltWorkspaceBuildStage: proto.PrebuiltWorkspaceBuildStage_CLAIM,
				},
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "null_resource",
					Metadata: []*proto.Resource_Metadata{{
						Key:   "is_prebuild_claim",
						Value: "true",
					}},
				}},
			},
		},
		{
			Name: "external-agent",
			Files: map[string]string{
				"main.tf": `terraform {
					required_providers {
					  coder = {
						source  = "coder/coder"
						version = ">= 2.7.0"
					  }
					}
				}
				resource "coder_external_agent" "example" {
					agent_id = "123"
				}
				`,
			},
			Response: &proto.GraphComplete{
				Resources: []*proto.Resource{{
					Name: "example",
					Type: "coder_external_agent",
				}},
				HasExternalAgents: true,
			},
		},
		{
			Name: "malicious-tar",
			Files: map[string]string{
				// Non-local path outside the working directory.
				"../../../etc/passwd": "content",
			},
			InitErrorContains: "refusing to extract to non-local path",
		},
	}

	// Remove unused cache dirs before running tests.
	// This cleans up any cache dirs that were created by tests that no longer exist.
	cacheRootDir := filepath.Join(testutil.PersistentCacheDir(t), "terraform_provision_test")
	expectedCacheDirs := make(map[string]bool)
	for _, testCase := range testCases {
		cacheDir := testutil.GetTestTFCacheDir(t, cacheRootDir, testCase.Name, testCase.Files)
		expectedCacheDirs[cacheDir] = true
	}
	currentCacheDirs, err := filepath.Glob(filepath.Join(cacheRootDir, "*"))
	require.NoError(t, err)
	for _, cacheDir := range currentCacheDirs {
		if _, ok := expectedCacheDirs[cacheDir]; !ok {
			t.Logf("removing unused cache dir: %s", cacheDir)
			require.NoError(t, os.RemoveAll(cacheDir))
		}
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			if testCase.SkipReason != "" {
				t.Skip(testCase.SkipReason)
			}

			cliConfigPath := ""
			if !testCase.SkipCacheProviders {
				cliConfigPath = testutil.CacheTFProviders(
					t,
					cacheRootDir,
					testCase.Name,
					testCase.Files,
				)
			}
			ctx, api := setupProvisioner(t, &provisionerServeOptions{
				cliConfigPath: cliConfigPath,
			})
			sess := configure(ctx, t, api, &proto.Config{})
			initLogGot := testCase.InitExpectLogContains == ""
			initComplete := sendInitAndGetResp(t, sess, testutil.CreateTar(t, testCase.Files), func(log string) {
				if strings.Contains(log, testCase.InitExpectLogContains) {
					initLogGot = true
				}
			})
			require.Truef(t, initLogGot, "did not get expected init log substring %q", testCase.InitExpectLogContains)
			if testCase.InitErrorContains != "" {
				require.Contains(t, initComplete.Error, testCase.InitErrorContains)
				return
			}
			require.Empty(t, initComplete.Error, "unexpected init error")

			planRequest := &proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
				Metadata: testCase.Metadata,
			}}}
			if testCase.Request != nil {
				planRequest = &proto.Request{Type: &proto.Request_Plan{Plan: testCase.Request}}
			}

			gotExpectedLog := testCase.PlanExpectLogContains == ""

			provision := func(req *proto.Request) *proto.Response {
				err := sess.Send(req)
				require.NoError(t, err)
				for {
					msg, err := sess.Recv()
					require.NoError(t, err)
					if msg.GetLog() != nil {
						if testCase.PlanExpectLogContains != "" && strings.Contains(msg.GetLog().Output, testCase.PlanExpectLogContains) {
							gotExpectedLog = true
						}

						t.Logf("log: [%s] %s", msg.GetLog().Level, msg.GetLog().Output)
						continue
					}
					return msg
				}
			}

			resp := provision(planRequest)
			planComplete := resp.GetPlan()
			require.NotNil(t, planComplete)

			if testCase.PlanErrorContains != "" {
				require.Contains(t, planComplete.GetError(), testCase.PlanErrorContains)
			}

			graphCompleteResp := provision(&proto.Request{Type: &proto.Request_Graph{Graph: &proto.GraphRequest{
				Source: proto.GraphSource_SOURCE_PLAN,
			}}})
			graphComplete := graphCompleteResp.GetGraph()
			require.NotNil(t, graphCompleteResp)

			if testCase.Response != nil {
				require.Equal(t, testCase.Response.Error, graphComplete.Error)

				// Remove randomly generated data and sort by name.
				normalizeResources(graphComplete.Resources)
				resourcesGot, err := json.Marshal(graphComplete.Resources)
				require.NoError(t, err)
				resourcesWant, err := json.Marshal(testCase.Response.Resources)
				require.NoError(t, err)
				require.Equal(t, string(resourcesWant), string(resourcesGot))

				parametersGot, err := json.Marshal(graphComplete.Parameters)
				require.NoError(t, err)
				parametersWant, err := json.Marshal(testCase.Response.Parameters)
				require.NoError(t, err)
				require.Equal(t, string(parametersWant), string(parametersGot))

				modulesGot, err := json.Marshal(initComplete.Modules)
				require.NoError(t, err)
				if testCase.InitResponse != nil {
					modulesWant, err := json.Marshal(testCase.InitResponse.Modules)
					require.NoError(t, err)
					require.Equal(t, string(modulesWant), string(modulesGot))
				}

				require.Equal(t, graphComplete.HasExternalAgents, testCase.Response.HasExternalAgents)
			}

			if testCase.Apply {
				resp = provision(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{
					Metadata: &proto.Metadata{WorkspaceTransition: proto.WorkspaceTransition_START},
				}}})
				applyComplete := resp.GetApply()
				require.NotNil(t, applyComplete)

				if testCase.Response != nil {
					normalizeResources(graphComplete.Resources)
					resourcesGot, err := json.Marshal(graphComplete.Resources)
					require.NoError(t, err)
					resourcesWant, err := json.Marshal(testCase.Response.Resources)
					require.NoError(t, err)
					require.Equal(t, string(resourcesWant), string(resourcesGot))
				}
			}

			if !gotExpectedLog {
				t.Fatalf("expected log string %q but never saw it", testCase.PlanExpectLogContains)
			}
		})
	}
}

func normalizeResources(resources []*proto.Resource) {
	for _, resource := range resources {
		sort.Slice(resource.Agents, func(i, j int) bool {
			return resource.Agents[i].Name < resource.Agents[j].Name
		})

		for _, agent := range resource.Agents {
			agent.Id = ""
			if agent.GetToken() == "" {
				continue
			}
			agent.Auth = &proto.Agent_Token{}
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Name < resources[j].Name
	})
}

// nolint:paralleltest
func TestProvision_ExtraEnv(t *testing.T) {
	// #nosec
	const secretValue = "oinae3uinxase"
	t.Setenv("TF_LOG", "INFO")
	t.Setenv("TF_SUPERSECRET", secretValue)

	ctx, api := setupProvisioner(t, nil)
	sess := configure(ctx, t, api, &proto.Config{})

	resp := sendInitAndGetResp(t, sess, testutil.CreateTar(t, map[string]string{"main.tf": `resource "null_resource" "A" {}`}))
	require.Empty(t, resp.Error)

	err := sendPlan(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)
	found := false
	for {
		msg, err := sess.Recv()
		require.NoError(t, err)

		if log := msg.GetLog(); log != nil {
			t.Log(log.Level.String(), log.Output)
			if strings.Contains(log.Output, "TF_LOG") {
				found = true
			}
			require.NotContains(t, log.Output, secretValue)
		}
		if c := msg.GetPlan(); c != nil {
			require.Empty(t, c.Error)
			break
		}
	}
	require.True(t, found)
}

// nolint:paralleltest
func TestProvision_SafeEnv(t *testing.T) {
	// #nosec
	const (
		passedValue = "superautopets"
		secretValue = "oinae3uinxase"
	)

	t.Setenv("VALID_USER_ENV", passedValue)

	// We ensure random CODER_ variables aren't passed through to avoid leaking
	// control plane secrets (e.g. PG URL).
	t.Setenv("CODER_SECRET", secretValue)

	const echoResource = `
	resource "null_resource" "a" {
		provisioner "local-exec" {
		  command = "env"
		}
	  }

	`

	ctx, api := setupProvisioner(t, nil)
	sess := configure(ctx, t, api, &proto.Config{})

	resp := sendInitAndGetResp(t, sess, testutil.CreateTar(t, map[string]string{"main.tf": echoResource}))
	require.Empty(t, resp.Error)

	err := sendPlan(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	_, _ = readProvisionLog(t, sess)

	err = sendApply(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	log, applyComplete := readProvisionLog(t, sess)
	require.Contains(t, log, passedValue)
	require.NotContains(t, log, secretValue)
	require.Contains(t, log, "CODER_")
	require.Contains(t, log, "AWS_SDK_UA_APP_ID=APN_1.1/pc_cdfmjwn8i6u8l9fwz8h82e4w3$")

	apply := applyComplete.Type.(*proto.Response_Apply)
	require.NotEmpty(t, apply.Apply.State, "state exists")
}

func TestProvision_MalformedModules(t *testing.T) {
	t.Parallel()

	ctx, api := setupProvisioner(t, nil)
	sess := configure(ctx, t, api, &proto.Config{})

	err := sendInit(sess, testutil.CreateTar(t, map[string]string{
		"main.tf":          `module "hello" { source = "./module" }`,
		"module/module.tf": `resource "null_`,
	}))
	require.NoError(t, err)

	log, _ := readProvisionLog(t, sess)
	require.Contains(t, log, "Invalid block definition")
}
