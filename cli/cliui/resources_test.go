package cliui_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceResources(t *testing.T) {
	t.Parallel()
	t.Run("SingleAgentSSH", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		ptty := ptytest.New(t)
		done := make(chan struct{})
		go func() {
			err := cliui.WorkspaceResources(ptty.Output(), []codersdk.WorkspaceResource{{
				Type:       "google_compute_instance",
				Name:       "dev",
				Transition: codersdk.WorkspaceTransitionStart,
				Agents: []codersdk.WorkspaceAgent{{
					Name:            "dev",
					Status:          codersdk.WorkspaceAgentConnected,
					LifecycleState:  codersdk.WorkspaceAgentLifecycleCreated,
					Architecture:    "amd64",
					OperatingSystem: "linux",
					Health:          codersdk.WorkspaceAgentHealth{Healthy: true},
				}},
			}}, cliui.WorkspaceResourcesOptions{
				WorkspaceName: "example",
			})
			assert.NoError(t, err)
			close(done)
		}()
		ptty.ExpectMatch(ctx, "coder ssh example")
		<-done
	})

	t.Run("MultipleStates", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		ptty := ptytest.New(t)
		disconnected := dbtime.Now().Add(-4 * time.Second)
		done := make(chan struct{})
		go func() {
			err := cliui.WorkspaceResources(ptty.Output(), []codersdk.WorkspaceResource{{
				Transition: codersdk.WorkspaceTransitionStart,
				Type:       "google_compute_disk",
				Name:       "root",
			}, {
				Transition: codersdk.WorkspaceTransitionStop,
				Type:       "google_compute_disk",
				Name:       "root",
			}, {
				Transition: codersdk.WorkspaceTransitionStart,
				Type:       "google_compute_instance",
				Name:       "dev",
				Agents: []codersdk.WorkspaceAgent{{
					CreatedAt:       dbtime.Now().Add(-10 * time.Second),
					Status:          codersdk.WorkspaceAgentConnecting,
					LifecycleState:  codersdk.WorkspaceAgentLifecycleCreated,
					Name:            "dev",
					OperatingSystem: "linux",
					Architecture:    "amd64",
					Health:          codersdk.WorkspaceAgentHealth{Healthy: true},
				}},
			}, {
				Transition: codersdk.WorkspaceTransitionStart,
				Type:       "kubernetes_pod",
				Name:       "dev",
				Agents: []codersdk.WorkspaceAgent{{
					Status:          codersdk.WorkspaceAgentConnected,
					LifecycleState:  codersdk.WorkspaceAgentLifecycleReady,
					Name:            "go",
					Architecture:    "amd64",
					OperatingSystem: "linux",
					Health:          codersdk.WorkspaceAgentHealth{Healthy: true},
				}, {
					DisconnectedAt:  &disconnected,
					Status:          codersdk.WorkspaceAgentDisconnected,
					LifecycleState:  codersdk.WorkspaceAgentLifecycleReady,
					Name:            "postgres",
					Architecture:    "amd64",
					OperatingSystem: "linux",
					Health: codersdk.WorkspaceAgentHealth{
						Healthy: false,
						Reason:  "agent has lost connection",
					},
				}},
			}}, cliui.WorkspaceResourcesOptions{
				WorkspaceName:  "dev",
				HideAgentState: false,
				HideAccess:     false,
			})
			assert.NoError(t, err)
			close(done)
		}()
		ptty.ExpectMatch(ctx, "google_compute_disk.root")
		ptty.ExpectMatch(ctx, "google_compute_instance.dev")
		ptty.ExpectMatch(ctx, "healthy")
		ptty.ExpectMatch(ctx, "coder ssh dev.dev")
		ptty.ExpectMatch(ctx, "kubernetes_pod.dev")
		ptty.ExpectMatch(ctx, "healthy")
		ptty.ExpectMatch(ctx, "coder ssh dev.go")
		ptty.ExpectMatch(ctx, "agent has lost connection")
		ptty.ExpectMatch(ctx, "coder ssh dev.postgres")
		<-done
	})
}
