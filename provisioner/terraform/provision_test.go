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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

type provisionerServeOptions struct {
	binaryPath  string
	exitTimeout time.Duration
	workDir     string
	logger      *slog.Logger
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
	client, server := drpc.MemTransportPipe()
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
			BinaryPath:  opts.binaryPath,
			CachePath:   cachePath,
			ExitTimeout: opts.exitTimeout,
		})
	}()
	api := proto.NewDRPCProvisionerClient(client)

	return ctx, api
}

func configure(ctx context.Context, t *testing.T, client proto.DRPCProvisionerClient, config *proto.Config) proto.DRPCProvisioner_SessionClient {
	t.Helper()
	sess, err := client.Session(ctx)
	require.NoError(t, err)
	err = sess.Send(&proto.Request{Type: &proto.Request_Config{Config: config}})
	require.NoError(t, err)
	return sess
}

func readProvisionLog(t *testing.T, response proto.DRPCProvisioner_SessionClient) string {
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
		break
	}
	return logBuf.String()
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
		tt := tt
		// below we exec fake_cancel.sh, which causes the kernel to execute it, and if more than
		// one process tries to do this, it can cause "text file busy"
		// nolint: paralleltest
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			binPath := filepath.Join(dir, "terraform")

			// Example: exec /path/to/terrafork_fake_cancel.sh 1.2.1 apply "$@"
			content := fmt.Sprintf("#!/bin/sh\nexec %q %s %s \"$@\"\n", fakeBin, terraform.TerraformVersion.String(), tt.mode)
			err := os.WriteFile(binPath, []byte(content), 0o755) //#nosec
			require.NoError(t, err)
			t.Logf("wrote fake terraform script to %s", binPath)

			ctx, api := setupProvisioner(t, &provisionerServeOptions{
				binaryPath: binPath,
			})
			sess := configure(ctx, t, api, &proto.Config{
				TemplateSourceArchive: testutil.CreateTar(t, nil),
			})

			err = sendPlan(sess, proto.WorkspaceTransition_START)
			require.NoError(t, err)

			for _, line := range tt.startSequence {
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
				}
				if c := msg.GetPlan(); c != nil {
					require.Contains(t, c.Error, "exit status 1")
					break
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

	sess := configure(ctx, t, api, &proto.Config{
		TemplateSourceArchive: testutil.CreateTar(t, nil),
	})

	// provisioner requires plan before apply, so test cancel with plan.
	err = sendPlan(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	for _, line := range []string{"init", "plan_start"} {
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

	sess := configure(ctx, t, api, &proto.Config{
		TemplateSourceArchive: testutil.CreateTar(t, nil),
	})

	err = sendPlan(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	found := false
	for {
		msg, err := sess.Recv()
		require.NoError(t, err)

		if c := msg.GetPlan(); c != nil {
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
		Response *proto.PlanComplete
		// If ErrorContains is not empty, PlanComplete should have an Error containing the given string
		ErrorContains string
		// If ExpectLogContains is not empty, then the logs should contain it.
		ExpectLogContains string
		// If Apply is true, then send an Apply request and check we get the same Resources as in Response.
		Apply bool
		// Some tests may need to be skipped until the relevant provider version is released.
		SkipReason string
	}{
		{
			Name: "missing-variable",
			Files: map[string]string{
				"main.tf": `variable "A" {
			}`,
			},
			ErrorContains:     "terraform plan:",
			ExpectLogContains: "No value for required variable",
		},
		{
			Name: "missing-variable-dry-run",
			Files: map[string]string{
				"main.tf": `variable "A" {
			}`,
			},
			ErrorContains:     "terraform plan:",
			ExpectLogContains: "No value for required variable",
		},
		{
			Name: "single-resource-dry-run",
			Files: map[string]string{
				"main.tf": `resource "null_resource" "A" {}`,
			},
			Response: &proto.PlanComplete{
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
			Response: &proto.PlanComplete{
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
			Response: &proto.PlanComplete{
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
			ErrorContains:     "initialize terraform",
			ExpectLogContains: "Argument or block definition required",
		},
		{
			Name: "bad-syntax-2",
			Files: map[string]string{
				"main.tf": `;asdf;`,
			},
			ErrorContains:     "initialize terraform",
			ExpectLogContains: `The ";" character is not valid.`,
		},
		{
			Name: "destroy-no-state",
			Files: map[string]string{
				"main.tf": `resource "null_resource" "A" {}`,
			},
			Metadata: &proto.Metadata{
				WorkspaceTransition: proto.WorkspaceTransition_DESTROY,
			},
			ExpectLogContains: "nothing to do",
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
			Response: &proto.PlanComplete{
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
			Response: &proto.PlanComplete{
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
			Response: &proto.PlanComplete{
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
			Response: &proto.PlanComplete{
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
			Response: &proto.PlanComplete{
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
			Response: &proto.PlanComplete{
				Resources: []*proto.Resource{{
					Name:       "example",
					Type:       "null_resource",
					ModulePath: "module.hello",
				}, {
					Name:       "inner_example",
					Type:       "null_resource",
					ModulePath: "module.hello.module.there",
				}},
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
			Response: &proto.PlanComplete{
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
						version = "2.3.0-pre2"
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
					Prebuild: true,
				},
			},
			Response: &proto.PlanComplete{
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
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			if testCase.SkipReason != "" {
				t.Skip(testCase.SkipReason)
			}

			ctx, api := setupProvisioner(t, nil)
			sess := configure(ctx, t, api, &proto.Config{
				TemplateSourceArchive: testutil.CreateTar(t, testCase.Files),
			})

			planRequest := &proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
				Metadata: testCase.Metadata,
			}}}
			if testCase.Request != nil {
				planRequest = &proto.Request{Type: &proto.Request_Plan{Plan: testCase.Request}}
			}

			gotExpectedLog := testCase.ExpectLogContains == ""

			provision := func(req *proto.Request) *proto.Response {
				err := sess.Send(req)
				require.NoError(t, err)
				for {
					msg, err := sess.Recv()
					require.NoError(t, err)
					if msg.GetLog() != nil {
						if testCase.ExpectLogContains != "" && strings.Contains(msg.GetLog().Output, testCase.ExpectLogContains) {
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

			if testCase.ErrorContains != "" {
				require.Contains(t, planComplete.GetError(), testCase.ErrorContains)
			}

			if testCase.Response != nil {
				require.Equal(t, testCase.Response.Error, planComplete.Error)

				// Remove randomly generated data and sort by name.
				normalizeResources(planComplete.Resources)
				resourcesGot, err := json.Marshal(planComplete.Resources)
				require.NoError(t, err)
				resourcesWant, err := json.Marshal(testCase.Response.Resources)
				require.NoError(t, err)
				require.Equal(t, string(resourcesWant), string(resourcesGot))

				parametersGot, err := json.Marshal(planComplete.Parameters)
				require.NoError(t, err)
				parametersWant, err := json.Marshal(testCase.Response.Parameters)
				require.NoError(t, err)
				require.Equal(t, string(parametersWant), string(parametersGot))

				modulesGot, err := json.Marshal(planComplete.Modules)
				require.NoError(t, err)
				modulesWant, err := json.Marshal(testCase.Response.Modules)
				require.NoError(t, err)
				require.Equal(t, string(modulesWant), string(modulesGot))
			}

			if testCase.Apply {
				resp = provision(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{
					Metadata: &proto.Metadata{WorkspaceTransition: proto.WorkspaceTransition_START},
				}}})
				applyComplete := resp.GetApply()
				require.NotNil(t, applyComplete)

				if testCase.Response != nil {
					normalizeResources(applyComplete.Resources)
					resourcesGot, err := json.Marshal(applyComplete.Resources)
					require.NoError(t, err)
					resourcesWant, err := json.Marshal(testCase.Response.Resources)
					require.NoError(t, err)
					require.Equal(t, string(resourcesWant), string(resourcesGot))
				}
			}

			if !gotExpectedLog {
				t.Fatalf("expected log string %q but never saw it", testCase.ExpectLogContains)
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
	sess := configure(ctx, t, api, &proto.Config{
		TemplateSourceArchive: testutil.CreateTar(t, map[string]string{"main.tf": `resource "null_resource" "A" {}`}),
	})

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
	sess := configure(ctx, t, api, &proto.Config{
		TemplateSourceArchive: testutil.CreateTar(t, map[string]string{"main.tf": echoResource}),
	})

	err := sendPlan(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	_ = readProvisionLog(t, sess)

	err = sendApply(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)

	log := readProvisionLog(t, sess)
	require.Contains(t, log, passedValue)
	require.NotContains(t, log, secretValue)
	require.Contains(t, log, "CODER_")
}

func TestProvision_MalformedModules(t *testing.T) {
	t.Parallel()

	ctx, api := setupProvisioner(t, nil)
	sess := configure(ctx, t, api, &proto.Config{
		TemplateSourceArchive: testutil.CreateTar(t, map[string]string{
			"main.tf":          `module "hello" { source = "./module" }`,
			"module/module.tf": `resource "null_`,
		}),
	})

	err := sendPlan(sess, proto.WorkspaceTransition_START)
	require.NoError(t, err)
	log := readProvisionLog(t, sess)
	require.Contains(t, log, "Invalid block definition")
}
