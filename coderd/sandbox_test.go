package coderd_test

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisioner/sandbox"
	"github.com/coder/coder/v2/provisionerd"
	provisionerdproto "github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

const sandboxTemplateYAML = `version: 1
image: coder/sandbox@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
cpu: 1
memory_mib: 2048
workdir: /workspace
daily_cost: 1
`

// This runtime exercises native job routing, state, and agent registration
// without requiring a Linux host. Runtime isolation is tested separately.
type sandboxTestRuntime struct {
	mu          sync.Mutex
	allocations map[string]sandbox.RuntimeSpec
	starts      int
}

func (r *sandboxTestRuntime) Start(_ context.Context, spec sandbox.RuntimeSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.allocations[spec.ID] = spec
	r.starts++
	return nil
}

func (r *sandboxTestRuntime) Inspect(_ context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, exists := r.allocations[id]
	return exists, nil
}

func (r *sandboxTestRuntime) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.allocations, id)
	return nil
}

func (r *sandboxTestRuntime) List(_ context.Context, workspaceID string) ([]sandbox.RuntimeAllocation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var allocations []sandbox.RuntimeAllocation
	for _, spec := range r.allocations {
		if workspaceID == "" || workspaceID == spec.WorkspaceID {
			allocations = append(allocations, sandbox.RuntimeAllocation{
				ID: spec.ID, WorkspaceID: spec.WorkspaceID, BuildID: spec.BuildID, BuildNumber: spec.BuildNumber, Image: spec.Image,
			})
		}
	}
	return allocations, nil
}

func (*sandboxTestRuntime) Close() error { return nil }

func newSandboxTestDaemon(t *testing.T, api *coderd.API) *sandboxTestRuntime {
	t.Helper()
	workDir := t.TempDir()
	stateDir := t.TempDir()
	runtime := &sandboxTestRuntime{allocations: make(map[string]sandbox.RuntimeSpec)}
	client, server := drpcsdk.MemTransportPipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- sandbox.Serve(ctx, &sandbox.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Listener:      server,
				WorkDirectory: workDir,
				Logger:        api.Logger.Named("sandbox"),
			},
			Runtime:        runtime,
			StateDirectory: stateDir,
		})
	}()
	t.Cleanup(func() {
		cancel()
		_ = client.Close()
		_ = server.Close()
		require.NoError(t, testutil.RequireReceive(testutil.Context(t, testutil.WaitLong), t, done))
	})
	connected := make(chan struct{})
	daemon := provisionerd.New(func(ctx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
		return api.CreateInMemoryTaggedProvisionerDaemon(ctx, uuid.NewString(),
			[]codersdk.ProvisionerType{codersdk.ProvisionerTypeSandbox},
			map[string]string{sandbox.HostTag: sandbox.HostID})
	}, &provisionerd.Options{
		Logger:         api.Logger.Named("sandbox-provisionerd"),
		UpdateInterval: 250 * time.Millisecond,
		Connector: provisionerd.LocalProvisioners{
			string(database.ProvisionerTypeSandbox): proto.NewDRPCProvisionerClient(client),
		},
		InitConnectionCh: connected,
	})
	t.Cleanup(func() { require.NoError(t, daemon.Close()) })
	select {
	case <-connected:
	case <-testutil.Context(t, testutil.WaitLong).Done():
		t.Fatal("sandbox provisioner did not connect")
	}
	return runtime
}

func TestSandboxTemplateLifecycle(t *testing.T) {
	t.Parallel()
	for _, classic := range []bool{false, true} {
		name := "Dynamic"
		if classic {
			name = "Classic"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, _, api := coderdtest.NewWithAPI(t, nil)
			owner := coderdtest.CreateFirstUser(t, client)
			runtime := newSandboxTestDaemon(t, api)
			ctx := testutil.Context(t, testutil.WaitLong)
			// Invalid HCL proves neither import nor builds invoke Terraform
			// parsing, in either parameter flow.
			manifest := sandboxTemplateYAML
			if !classic {
				// The real agent runs locally for this connection test; only
				// compute is simulated, so its working directory must exist.
				manifest = strings.Replace(manifest, "workdir: /workspace", "workdir: "+t.TempDir(), 1)
			}
			responses := &echo.Responses{ExtraFiles: map[string][]byte{
				"sandbox.yaml": []byte(manifest),
				"main.tf":      []byte("this is not valid Terraform {{{"),
			}}
			version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, responses,
				func(req *codersdk.CreateTemplateVersionRequest) { req.Provisioner = codersdk.ProvisionerTypeSandbox })
			version = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			require.Equal(t, codersdk.ProvisionerJobSucceeded, version.Job.Status)
			require.Empty(t, version.Warnings)
			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID,
				func(req *codersdk.CreateTemplateRequest) { req.UseClassicParameterFlow = &classic })
			require.Equal(t, codersdk.ProvisionerTypeSandbox, template.Provisioner)

			// Updating an existing template must bypass classic parsing too.
			version = coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, responses,
				func(req *codersdk.CreateTemplateVersionRequest) {
					req.Provisioner = codersdk.ProvisionerTypeSandbox
					req.TemplateID = template.ID
				})
			version = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
			coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, version.ID)
			parameters, err := client.TemplateVersionRichParameters(ctx, version.ID)
			require.NoError(t, err)
			require.Empty(t, parameters)
			stream, err := client.TemplateVersionDynamicParameters(ctx, codersdk.Me, version.ID)
			require.NoError(t, err)
			preview := testutil.RequireReceive(ctx, t, stream.Chan())
			require.Empty(t, preview.Diagnostics)
			require.Empty(t, preview.Parameters)
			require.NoError(t, stream.Close(1000))

			_, err = client.CreateTemplateVersionDryRun(ctx, version.ID, codersdk.CreateTemplateVersionDryRunRequest{
				RichParameterValues: []codersdk.WorkspaceBuildParameter{{Name: "cpu", Value: "64"}},
			})
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())

			dryRun, err := client.CreateTemplateVersionDryRun(ctx, version.ID, codersdk.CreateTemplateVersionDryRunRequest{})
			require.NoError(t, err)
			require.Eventually(t, func() bool {
				job, err := client.TemplateVersionDryRun(ctx, version.ID, dryRun.ID)
				return err == nil && job.Status == codersdk.ProvisionerJobSucceeded
			}, testutil.WaitLong, testutil.IntervalFast)
			runtime.mu.Lock()
			starts := runtime.starts
			runtime.mu.Unlock()
			require.Zero(t, starts, "template import and dry-run must not create compute")
			workspace := coderdtest.CreateWorkspace(t, client, template.ID)
			completed := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)
			require.Equal(t, codersdk.ProvisionerJobSucceeded, completed.Job.Status)
			require.Len(t, completed.Resources, 1)
			require.Len(t, completed.Resources[0].Agents, 1)
			runtime.mu.Lock()
			var firstSpec sandbox.RuntimeSpec
			for _, spec := range runtime.allocations {
				firstSpec = spec
			}
			runtime.mu.Unlock()
			require.Equal(t, workspace.ID.String(), firstSpec.WorkspaceID)
			require.Equal(t, completed.ID.String(), firstSpec.BuildID)
			require.Equal(t, int64(2*1024*1024*1024), firstSpec.MemoryBytes)
			timings, err := client.WorkspaceBuildTimings(ctx, completed.ID)
			require.NoError(t, err)
			var runtimeTimings []codersdk.ProvisionerTiming
			for _, timing := range timings.ProvisionerTimings {
				if timing.Stage == codersdk.TimingStageApply && timing.Source == "containerd" && timing.Action == "create" {
					runtimeTimings = append(runtimeTimings, timing)
				}
			}
			require.Len(t, runtimeTimings, 1, "runtime creation must be measured separately from lifecycle overhead")
			require.Equal(t, firstSpec.ID, runtimeTimings[0].Resource)
			require.False(t, runtimeTimings[0].EndedAt.Before(runtimeTimings[0].StartedAt))
			agent, err := api.Database.GetWorkspaceAgentByID(dbauthz.AsSystemRestricted(ctx), completed.Resources[0].Agents[0].ID)
			require.NoError(t, err)
			require.Equal(t, firstSpec.AgentToken, agent.AuthToken.String())
			if !classic {
				assertSandboxSSH(t, client, workspace.ID, firstSpec.AgentToken)
			}

			job, err := api.Database.GetProvisionerJobByID(dbauthz.AsSystemRestricted(ctx), workspace.LatestBuild.Job.ID)
			require.NoError(t, err)
			require.Equal(t, database.ProvisionerTypeSandbox, job.Provisioner)
			require.Equal(t, sandbox.HostID, job.Tags[sandbox.HostTag])

			_, err = client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition:          codersdk.WorkspaceTransitionStart,
				RichParameterValues: []codersdk.WorkspaceBuildParameter{{Name: "cpu", Value: "64"}},
			})
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())

			_, err = client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionDelete, Orphan: true,
			})
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
			unchanged, err := client.Workspace(ctx, workspace.ID)
			require.NoError(t, err)
			require.Equal(t, workspace.LatestBuild.ID, unchanged.LatestBuild.ID)

			for _, transition := range []codersdk.WorkspaceTransition{
				codersdk.WorkspaceTransitionStop, codersdk.WorkspaceTransitionStart, codersdk.WorkspaceTransitionDelete,
			} {
				build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{Transition: transition})
				require.NoError(t, err)
				completed := coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, build.ID)
				require.Equal(t, codersdk.ProvisionerJobSucceeded, completed.Job.Status)
				runtime.mu.Lock()
				var currentSpec sandbox.RuntimeSpec
				for _, spec := range runtime.allocations {
					currentSpec = spec
				}
				runtime.mu.Unlock()
				if transition == codersdk.WorkspaceTransitionStart {
					require.NotEmpty(t, currentSpec.ID)
					require.NotEqual(t, firstSpec.ID, currentSpec.ID)
					require.NotEqual(t, firstSpec.AgentToken, currentSpec.AgentToken)
				} else {
					require.Empty(t, currentSpec.ID)
				}
			}
			runtime.mu.Lock()
			allocationCount, starts := len(runtime.allocations), runtime.starts
			runtime.mu.Unlock()
			require.Zero(t, allocationCount)
			require.Equal(t, 2, starts, "restart must create a new sandbox")
		})
	}
}

// assertSandboxSSH runs the real agent and authenticates through Coder using
// the credential created by the native provisioner. Compute remains simulated.
func assertSandboxSSH(t *testing.T, client *codersdk.Client, workspaceID uuid.UUID, token string) {
	t.Helper()
	localAgent := agenttest.New(t, client.URL, token)
	defer localAgent.Close()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspaceID)
	ctx := testutil.Context(t, testutil.WaitLong)
	conn, err := workspacesdk.New(client).DialAgent(ctx, resources[0].Agents[0].ID,
		&workspacesdk.DialAgentOptions{Logger: testutil.Logger(t).Named("sandbox-ssh")})
	require.NoError(t, err)
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	sshClient, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	defer sshClient.Close()
	session, err := sshClient.NewSession()
	require.NoError(t, err)
	defer session.Close()
	require.NoError(t, session.Run("true"))
}

func TestSandboxTemplateValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		manifest string
		mutate   func(*codersdk.CreateTemplateVersionRequest)
	}{
		{name: "MutableImage", manifest: "version: 1\nimage: coder/sandbox:latest\n"},
		{name: "Variables", manifest: sandboxTemplateYAML, mutate: func(req *codersdk.CreateTemplateVersionRequest) {
			req.UserVariableValues = []codersdk.VariableValue{{Name: "cpu", Value: "64"}}
		}},
		{name: "HostOverride", manifest: sandboxTemplateYAML, mutate: func(req *codersdk.CreateTemplateVersionRequest) {
			req.ProvisionerTags = map[string]string{sandbox.HostTag: "another-host"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := coderdtest.New(t, nil)
			owner := coderdtest.CreateFirstUser(t, client)
			ctx := testutil.Context(t, testutil.WaitLong)
			archive, err := echo.Tar(&echo.Responses{ExtraFiles: map[string][]byte{"sandbox.yaml": []byte(tc.manifest)}})
			require.NoError(t, err)
			file, err := client.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(archive))
			require.NoError(t, err)
			req := codersdk.CreateTemplateVersionRequest{
				FileID: file.ID, StorageMethod: codersdk.ProvisionerStorageMethodFile, Provisioner: codersdk.ProvisionerTypeSandbox,
			}
			if tc.mutate != nil {
				tc.mutate(&req)
			}
			_, err = client.CreateTemplateVersion(ctx, owner.OrganizationID, req)
			var apiErr *codersdk.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		})
	}
}
