//go:build linux || darwin

package pulumi_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/provisioner/pulumi"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

const (
	fakePulumiPreviewJSON = `{"steps":[],"changeSummary":{"same":0}}`
	fakePulumiStateJSON   = `{
		"version": 3,
		"deployment": {
			"manifest": {
				"time": "2024-01-01T00:00:00Z",
				"magic": "abc",
				"version": "v3.100.0"
			},
			"resources": [
				{
					"urn": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
					"type": "pulumi:pulumi:Stack",
					"custom": false,
					"inputs": {},
					"outputs": {}
				},
				{
					"urn": "urn:pulumi:coder::project::docker:index/container:Container::workspace",
					"type": "docker:index/container:Container",
					"custom": true,
					"id": "container-abc123",
					"inputs": {
						"name": "workspace"
					},
					"outputs": {
						"id": "container-abc123",
						"name": "workspace"
					},
					"parent": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
					"dependencies": []
				},
				{
					"urn": "urn:pulumi:coder::project::coder:index/agent:Agent::dev",
					"type": "coder:index/agent:Agent",
					"custom": true,
					"inputs": {},
					"outputs": {
						"id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
						"auth": "token",
						"token": "test-token",
						"os": "linux",
						"arch": "amd64",
						"dir": "/workspace"
					},
					"parent": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
					"dependencies": [
						"urn:pulumi:coder::project::docker:index/container:Container::workspace"
					]
				},
				{
					"urn": "urn:pulumi:coder::project::coder:index/app:App::code-server",
					"type": "coder:index/app:App",
					"custom": true,
					"inputs": {},
					"outputs": {
						"id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
						"agentId": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
						"slug": "code-server",
						"displayName": "Code Server",
						"url": "http://localhost:8080"
					},
					"parent": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
					"dependencies": [
						"urn:pulumi:coder::project::coder:index/agent:Agent::dev"
					]
				},
				{
					"urn": "urn:pulumi:coder::project::coder:index/parameter:Parameter::region",
					"type": "coder:index/parameter:Parameter",
					"custom": true,
					"inputs": {},
					"outputs": {
						"name": "region",
						"type": "string",
						"default": "us-east-1",
						"description": "Cloud region"
					},
					"parent": "urn:pulumi:coder::project::pulumi:pulumi:Stack::project",
					"dependencies": []
				}
			]
		}
	}`
)

type provisionerServeOptions struct {
	binaryPath  string
	exitTimeout time.Duration
	workDir     string
	logger      *slog.Logger
}

func setupProvisioner(t *testing.T, opts *provisionerServeOptions) (context.Context, proto.DRPCProvisionerClient) {
	t.Helper()
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

	clientConn, serverConn := drpcsdk.MemTransportPipe()
	ctx, cancel := context.WithCancel(context.Background())
	serverErr := make(chan error, 1)
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		cancel()
		err := <-serverErr
		if !errors.Is(err, context.Canceled) {
			assert.NoError(t, err)
		}
	})

	go func() {
		serverErr <- pulumi.Serve(ctx, &pulumi.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Listener:      serverConn,
				Logger:        *opts.logger,
				WorkDirectory: opts.workDir,
			},
			BinaryPath:  opts.binaryPath,
			CachePath:   cachePath,
			ExitTimeout: opts.exitTimeout,
		})
	}()

	return ctx, proto.NewDRPCProvisionerClient(clientConn)
}

func configure(ctx context.Context, t *testing.T, client proto.DRPCProvisionerClient, config *proto.Config) proto.DRPCProvisioner_SessionClient {
	t.Helper()
	sess, err := client.Session(ctx)
	require.NoError(t, err)
	err = sess.Send(&proto.Request{Type: &proto.Request_Config{Config: config}})
	require.NoError(t, err)
	return sess
}

func sendInit(sess proto.DRPCProvisioner_SessionClient, archive []byte) error {
	return sess.Send(&proto.Request{Type: &proto.Request_Init{Init: &proto.InitRequest{
		TemplateSourceArchive: archive,
	}}})
}

func sendInitAndGetResp(t *testing.T, sess proto.DRPCProvisioner_SessionClient, archive []byte) *proto.InitComplete {
	t.Helper()
	err := sendInit(sess, archive)
	require.NoError(t, err)
	for {
		msg, err := sess.Recv()
		require.NoError(t, err)
		if msg.GetLog() != nil {
			continue
		}
		init := msg.GetInit()
		require.NotNil(t, init)
		return init
	}
}

func sendPlan(sess proto.DRPCProvisioner_SessionClient, transition proto.WorkspaceTransition) error {
	return sendPlanWithState(sess, transition, nil)
}

func sendPlanWithState(sess proto.DRPCProvisioner_SessionClient, transition proto.WorkspaceTransition, state []byte) error {
	return sess.Send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
		Metadata: &proto.Metadata{WorkspaceTransition: transition},
		State:    state,
	}}})
}

func sendApply(sess proto.DRPCProvisioner_SessionClient, transition proto.WorkspaceTransition) error {
	return sess.Send(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{
		Metadata: &proto.Metadata{WorkspaceTransition: transition},
	}}})
}

func sendGraph(sess proto.DRPCProvisioner_SessionClient, source proto.GraphSource) error {
	return sess.Send(&proto.Request{Type: &proto.Request_Graph{Graph: &proto.GraphRequest{Source: source}}})
}

func readProvisionLog(t *testing.T, response proto.DRPCProvisioner_SessionClient) (string, *proto.Response) {
	t.Helper()
	var last *proto.Response
	var logBuf []byte
	for {
		msg, err := response.Recv()
		require.NoError(t, err)
		if log := msg.GetLog(); log != nil {
			logBuf = append(logBuf, log.Output...)
			logBuf = append(logBuf, '\n')
			continue
		}
		last = msg
		break
	}
	return string(logBuf), last
}

func fakePulumiPath(t *testing.T, name string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(cwd, "testdata", name)
}

func minimalArchive(t *testing.T) []byte {
	t.Helper()
	return testutil.CreateTar(t, map[string]string{
		"Pulumi.yaml":  "name: test\nruntime: yaml\n",
		"package.json": "{\n  \"name\": \"test\",\n  \"version\": \"1.0.0\"\n}\n",
	})
}

func archiveWithPackages(t *testing.T) []byte {
	t.Helper()
	return testutil.CreateTar(t, map[string]string{
		"Pulumi.yaml": `name: test
runtime: yaml
packages:
  coder:
    source: terraform-provider
    version: 1.1.1
    parameters:
      - coder/coder
  docker:
    source: terraform-provider
    version: 1.1.1
    parameters:
      - kreuzwerker/docker
`,
		"package.json": "{\n  \"name\": \"test\",\n  \"version\": \"1.0.0\"\n}\n",
	})
}

func requireFakePulumiGraph(t *testing.T, graph *proto.GraphComplete) {
	t.Helper()

	require.Len(t, graph.Resources, 1)
	require.Equal(t, "workspace", graph.Resources[0].Name)
	require.Equal(t, "docker:index/container:Container", graph.Resources[0].Type)
	require.Len(t, graph.Resources[0].Agents, 1)

	agent := graph.Resources[0].Agents[0]
	require.Equal(t, "dev", agent.Name)
	require.Equal(t, "linux", agent.OperatingSystem)
	require.Equal(t, "amd64", agent.Architecture)
	require.Equal(t, "/workspace", agent.Directory)
	require.Len(t, agent.Apps, 1)
	require.Equal(t, "code-server", agent.Apps[0].Slug)
	require.Equal(t, "Code Server", agent.Apps[0].DisplayName)
	require.Equal(t, "http://localhost:8080", agent.Apps[0].Url)

	require.Len(t, graph.Parameters, 1)
	require.Equal(t, "region", graph.Parameters[0].Name)
	require.Equal(t, "string", graph.Parameters[0].Type)
	require.Equal(t, "us-east-1", graph.Parameters[0].DefaultValue)
	require.Equal(t, "Cloud region", graph.Parameters[0].Description)
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_VersionTooOld(t *testing.T) {
	clientConn, serverConn := drpcsdk.MemTransportPipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	err := pulumi.Serve(ctx, &pulumi.ServeOptions{
		ServeOptions: &provisionersdk.ServeOptions{
			Listener:      serverConn,
			Logger:        logger,
			WorkDirectory: t.TempDir(),
		},
		BinaryPath: fakePulumiPath(t, "fake_pulumi_bad_version.sh"),
		CachePath:  t.TempDir(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `pulumi version "2.0.0" is too old. required >= "3.0.0"`)
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_Init(t *testing.T) {
	workDir := t.TempDir()
	ctx, client := setupProvisioner(t, &provisionerServeOptions{
		binaryPath: fakePulumiPath(t, "fake_pulumi.sh"),
		workDir:    workDir,
	})
	sess := configure(ctx, t, client, &proto.Config{})

	resp := sendInitAndGetResp(t, sess, minimalArchive(t))
	require.Empty(t, resp.GetError())
	require.NotEmpty(t, resp.GetTimings())

	packageLocks, err := filepath.Glob(filepath.Join(workDir, "Session*", "package-lock.json"))
	require.NoError(t, err)
	require.Len(t, packageLocks, 1)
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_InitPackageAdd(t *testing.T) {
	workDir := t.TempDir()
	ctx, client := setupProvisioner(t, &provisionerServeOptions{
		binaryPath: fakePulumiPath(t, "fake_pulumi.sh"),
		workDir:    workDir,
	})
	sess := configure(ctx, t, client, &proto.Config{})

	resp := sendInitAndGetResp(t, sess, archiveWithPackages(t))
	require.Empty(t, resp.GetError())
	require.NotEmpty(t, resp.GetTimings())

	packageAddMarkers, err := filepath.Glob(filepath.Join(workDir, "Session*", ".pulumi-package-add-ran"))
	require.NoError(t, err)
	require.Len(t, packageAddMarkers, 1)

	packageAddLog, err := os.ReadFile(packageAddMarkers[0])
	require.NoError(t, err)
	require.Contains(t, string(packageAddLog), "package add terraform-provider coder/coder --non-interactive")
	require.Contains(t, string(packageAddLog), "package add terraform-provider kreuzwerker/docker --non-interactive")
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_ParseEmpty(t *testing.T) {
	ctx, client := setupProvisioner(t, &provisionerServeOptions{binaryPath: fakePulumiPath(t, "fake_pulumi.sh")})
	sess := configure(ctx, t, client, &proto.Config{})
	sendInitAndGetResp(t, sess, minimalArchive(t))

	err := sess.Send(&proto.Request{Type: &proto.Request_Parse{Parse: &proto.ParseRequest{}}})
	require.NoError(t, err)

	_, resp := readProvisionLog(t, sess)
	parse := resp.GetParse()
	require.NotNil(t, parse)
	require.Empty(t, parse.GetError())
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_PlanStart(t *testing.T) {
	ctx, client := setupProvisioner(t, &provisionerServeOptions{binaryPath: fakePulumiPath(t, "fake_pulumi.sh")})
	sess := configure(ctx, t, client, &proto.Config{})
	sendInitAndGetResp(t, sess, minimalArchive(t))

	err := sendPlan(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	logs, resp := readProvisionLog(t, sess)
	plan := resp.GetPlan()
	require.NotNil(t, plan)
	require.Empty(t, plan.GetError())
	require.NotEmpty(t, plan.GetTimings())
	require.JSONEq(t, fakePulumiPreviewJSON, string(plan.GetPlan()))
	require.Contains(t, logs, fakePulumiPreviewJSON)
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_PlanDestroy_NoState(t *testing.T) {
	ctx, client := setupProvisioner(t, &provisionerServeOptions{binaryPath: fakePulumiPath(t, "fake_pulumi.sh")})
	sess := configure(ctx, t, client, &proto.Config{})
	sendInitAndGetResp(t, sess, minimalArchive(t))

	err := sendPlan(sess, proto.WorkspaceTransition_DESTROY)
	require.NoError(t, err)

	logs, resp := readProvisionLog(t, sess)
	plan := resp.GetPlan()
	require.NotNil(t, plan)
	require.Empty(t, plan.GetError())
	require.Empty(t, plan.GetPlan())
	require.Contains(t, logs, "The Pulumi state does not exist, there is nothing to do")
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_PlanDestroy_WithState(t *testing.T) {
	ctx, client := setupProvisioner(t, &provisionerServeOptions{binaryPath: fakePulumiPath(t, "fake_pulumi.sh")})
	sess := configure(ctx, t, client, &proto.Config{})
	sendInitAndGetResp(t, sess, minimalArchive(t))

	err := sendPlanWithState(sess, proto.WorkspaceTransition_DESTROY, []byte(`{"version":3}`))
	require.NoError(t, err)

	logs, resp := readProvisionLog(t, sess)
	plan := resp.GetPlan()
	require.NotNil(t, plan)
	require.Empty(t, plan.GetError())
	require.NotEmpty(t, plan.GetTimings())
	require.JSONEq(t, fakePulumiPreviewJSON, string(plan.GetPlan()))
	require.Contains(t, logs, fakePulumiPreviewJSON)
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_Apply(t *testing.T) {
	ctx, client := setupProvisioner(t, &provisionerServeOptions{binaryPath: fakePulumiPath(t, "fake_pulumi.sh")})
	sess := configure(ctx, t, client, &proto.Config{})
	sendInitAndGetResp(t, sess, minimalArchive(t))
	require.NoError(t, sendPlan(sess, proto.WorkspaceTransition_START))
	_, _ = readProvisionLog(t, sess)

	err := sendApply(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	logs, resp := readProvisionLog(t, sess)
	apply := resp.GetApply()
	require.NotNil(t, apply)
	require.Empty(t, apply.GetError())
	require.NotEmpty(t, apply.GetTimings())
	require.JSONEq(t, fakePulumiStateJSON, string(apply.GetState()))
	require.Contains(t, logs, "Updating (coder):")
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_ApplyDestroy_NoState(t *testing.T) {
	ctx, client := setupProvisioner(t, &provisionerServeOptions{binaryPath: fakePulumiPath(t, "fake_pulumi.sh")})
	sess := configure(ctx, t, client, &proto.Config{})
	sendInitAndGetResp(t, sess, minimalArchive(t))
	require.NoError(t, sendPlan(sess, proto.WorkspaceTransition_DESTROY))
	_, _ = readProvisionLog(t, sess)

	err := sendApply(sess, proto.WorkspaceTransition_DESTROY)
	require.NoError(t, err)

	logs, resp := readProvisionLog(t, sess)
	apply := resp.GetApply()
	require.NotNil(t, apply)
	require.Empty(t, apply.GetError())
	require.Empty(t, apply.GetState())
	require.Contains(t, logs, "The Pulumi state does not exist, there is nothing to do")
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_Graph_FromPlan(t *testing.T) {
	ctx, client := setupProvisioner(t, &provisionerServeOptions{binaryPath: fakePulumiPath(t, "fake_pulumi.sh")})
	sess := configure(ctx, t, client, &proto.Config{})
	sendInitAndGetResp(t, sess, minimalArchive(t))
	require.NoError(t, sendPlan(sess, proto.WorkspaceTransition_START))
	_, _ = readProvisionLog(t, sess)

	err := sendGraph(sess, proto.GraphSource_SOURCE_PLAN)
	require.NoError(t, err)

	_, resp := readProvisionLog(t, sess)
	graph := resp.GetGraph()
	require.NotNil(t, graph)
	require.Empty(t, graph.GetError())
	require.Empty(t, graph.Resources)
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_Graph_FromPlanWithState(t *testing.T) {
	ctx, client := setupProvisioner(t, &provisionerServeOptions{binaryPath: fakePulumiPath(t, "fake_pulumi.sh")})
	sess := configure(ctx, t, client, &proto.Config{})
	sendInitAndGetResp(t, sess, minimalArchive(t))
	require.NoError(t, sendPlanWithState(sess, proto.WorkspaceTransition_START, []byte(fakePulumiStateJSON)))
	_, _ = readProvisionLog(t, sess)

	err := sendGraph(sess, proto.GraphSource_SOURCE_PLAN)
	require.NoError(t, err)

	_, resp := readProvisionLog(t, sess)
	graph := resp.GetGraph()
	require.NotNil(t, graph)
	require.Empty(t, graph.GetError())
	requireFakePulumiGraph(t, graph)
}

// nolint: paralleltest
// These tests exec the shared fake shell binary, which can trigger
// "text file busy" when run in parallel.
func TestProvision_Graph_FromState(t *testing.T) {
	ctx, client := setupProvisioner(t, &provisionerServeOptions{binaryPath: fakePulumiPath(t, "fake_pulumi.sh")})
	sess := configure(ctx, t, client, &proto.Config{})
	sendInitAndGetResp(t, sess, minimalArchive(t))

	err := sendGraph(sess, proto.GraphSource_SOURCE_STATE)
	require.NoError(t, err)

	_, resp := readProvisionLog(t, sess)
	graph := resp.GetGraph()
	require.NotNil(t, graph)
	require.Empty(t, graph.GetError())
	requireFakePulumiGraph(t, graph)
}
