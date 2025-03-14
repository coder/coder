package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/pretty"
	"github.com/coder/serpent"
)
func main() {
	var root *serpent.Command
	root = &serpent.Command{
		Use:   "cliui",

		Short: "Used for visually testing UI components for the CLI.",
		HelpHandler: func(inv *serpent.Invocation) error {
			_, _ = fmt.Fprintln(inv.Stdout, "This command is used for visually testing UI components for the CLI.")
			_, _ = fmt.Fprintln(inv.Stdout, "It is not intended to be used by end users.")
			_, _ = fmt.Fprintln(inv.Stdout, "Subcommands: ")
			for _, child := range root.Children {
				_, _ = fmt.Fprintf(inv.Stdout, "- %s\n", child.Use)
			}
			return nil
		},
	}
	root.Children = append(root.Children, &serpent.Command{
		Use:    "colors",
		Hidden: true,
		Handler: func(inv *serpent.Invocation) error {
			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.Code, "This is a code message")

			_, _ = fmt.Fprintln(inv.Stdout)
			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.DateTimeStamp, "This is a datetimestamp message")
			_, _ = fmt.Fprintln(inv.Stdout)
			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.Error, "This is an error message")
			_, _ = fmt.Fprintln(inv.Stdout)
			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.Field, "This is a field message")
			_, _ = fmt.Fprintln(inv.Stdout)

			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.Keyword, "This is a keyword message")
			_, _ = fmt.Fprintln(inv.Stdout)
			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.Placeholder, "This is a placeholder message")

			_, _ = fmt.Fprintln(inv.Stdout)
			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.Prompt, "This is a prompt message")
			_, _ = fmt.Fprintln(inv.Stdout)

			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.FocusedPrompt, "This is a focused prompt message")
			_, _ = fmt.Fprintln(inv.Stdout)
			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.Fuchsia, "This is a fuchsia message")

			_, _ = fmt.Fprintln(inv.Stdout)
			pretty.Fprintf(inv.Stdout, cliui.DefaultStyles.Warn, "This is a warning message")
			_, _ = fmt.Fprintln(inv.Stdout)

			return nil
		},
	})

	root.Children = append(root.Children, &serpent.Command{
		Use: "prompt",
		Handler: func(inv *serpent.Invocation) error {

			_, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:    "What is our " + cliui.Field("company name") + "?",
				Default: "acme-corp",

				Validate: func(s string) error {
					if !strings.EqualFold(s, "coder") {
						return errors.New("Err... nope!")

					}
					return nil
				},

			})
			if errors.Is(err, cliui.Canceled) {
				return nil
			}

			if err != nil {
				return err
			}
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Do you want to accept?",
				Default:   cliui.ConfirmYes,
				IsConfirm: true,
			})
			if errors.Is(err, cliui.Canceled) {
				return nil
			}
			if err != nil {
				return err
			}
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:   "Enter password",
				Secret: true,
			})
			return err
		},
	})
	root.Children = append(root.Children, &serpent.Command{
		Use: "select",
		Handler: func(inv *serpent.Invocation) error {
			value, err := cliui.Select(inv, cliui.SelectOptions{
				Options: []string{"Tomato", "Banana", "Onion", "Grape", "Lemon"},
				Size:    3,
			})
			_, _ = fmt.Printf("Selected: %q\n", value)
			return err
		},
	})
	root.Children = append(root.Children, &serpent.Command{
		Use: "job",
		Handler: func(inv *serpent.Invocation) error {
			job := codersdk.ProvisionerJob{
				Status:    codersdk.ProvisionerJobPending,
				CreatedAt: dbtime.Now(),

			}
			go func() {
				time.Sleep(time.Second)
				if job.Status != codersdk.ProvisionerJobPending {
					return
				}
				started := dbtime.Now()
				job.StartedAt = &started
				job.Status = codersdk.ProvisionerJobRunning
				time.Sleep(3 * time.Second)
				if job.Status != codersdk.ProvisionerJobRunning {
					return

				}
				completed := dbtime.Now()
				job.CompletedAt = &completed
				job.Status = codersdk.ProvisionerJobSucceeded
			}()
			err := cliui.ProvisionerJob(inv.Context(), inv.Stdout, cliui.ProvisionerJobOptions{
				Fetch: func() (codersdk.ProvisionerJob, error) {
					return job, nil
				},
				Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
					logs := make(chan codersdk.ProvisionerJobLog)
					go func() {
						defer close(logs)
						ticker := time.NewTicker(100 * time.Millisecond)
						defer ticker.Stop()
						count := 0
						for {
							select {
							case <-inv.Context().Done():
								return
							case <-ticker.C:
								if job.Status == codersdk.ProvisionerJobSucceeded || job.Status == codersdk.ProvisionerJobCanceled {
									return
								}

								log := codersdk.ProvisionerJobLog{
									CreatedAt: time.Now(),
									Output:    fmt.Sprintf("Some log %d", count),
									Level:     codersdk.LogLevelInfo,
								}
								switch {
								case count == 10:
									log.Stage = "Setting Up"
								case count == 20:
									log.Stage = "Executing Hook"
								case count == 30:
									log.Stage = "Parsing Variables"
								case count == 40:
									log.Stage = "Provisioning"
								case count == 50:
									log.Stage = "Cleaning Up"
								}
								if count%5 == 0 {
									log.Level = codersdk.LogLevelWarn
								}
								count++
								if log.Output == "" && log.Stage == "" {
									continue
								}
								logs <- log
							}
						}
					}()
					return logs, io.NopCloser(strings.NewReader("")), nil
				},
				Cancel: func() error {
					job.Status = codersdk.ProvisionerJobCanceling
					time.Sleep(time.Second)
					job.Status = codersdk.ProvisionerJobCanceled
					completed := dbtime.Now()
					job.CompletedAt = &completed
					return nil
				},
			})
			return err
		},
	})
	root.Children = append(root.Children, &serpent.Command{
		Use: "agent",
		Handler: func(inv *serpent.Invocation) error {
			var agent codersdk.WorkspaceAgent
			var logs []codersdk.WorkspaceAgentLog
			fetchSteps := []func(){
				func() {
					createdAt := time.Now().Add(-time.Minute)
					agent = codersdk.WorkspaceAgent{
						CreatedAt:      createdAt,
						Status:         codersdk.WorkspaceAgentConnecting,
						LifecycleState: codersdk.WorkspaceAgentLifecycleCreated,
					}
				},
				func() {
					time.Sleep(time.Second)
					agent.Status = codersdk.WorkspaceAgentTimeout
				},
				func() {
					agent.LifecycleState = codersdk.WorkspaceAgentLifecycleStarting

					startingAt := time.Now()
					agent.StartedAt = &startingAt
					for i := 0; i < 10; i++ {
						level := codersdk.LogLevelInfo
						if rand.Float64() > 0.75 { //nolint:gosec
							level = codersdk.LogLevelError

						}
						logs = append(logs, codersdk.WorkspaceAgentLog{
							CreatedAt: time.Now().Add(-time.Duration(10-i) * 144 * time.Millisecond),
							Output:    fmt.Sprintf("Some log %d", i),
							Level:     level,
						})
					}
				},
				func() {
					time.Sleep(time.Second)
					firstConnectedAt := time.Now()
					agent.FirstConnectedAt = &firstConnectedAt
					lastConnectedAt := firstConnectedAt.Add(0)
					agent.LastConnectedAt = &lastConnectedAt
					agent.Status = codersdk.WorkspaceAgentConnected
				},
				func() {},
				func() {
					time.Sleep(5 * time.Second)
					agent.Status = codersdk.WorkspaceAgentConnected
					lastConnectedAt := time.Now()
					agent.LastConnectedAt = &lastConnectedAt
				},
			}
			err := cliui.Agent(inv.Context(), inv.Stdout, uuid.Nil, cliui.AgentOptions{
				FetchInterval: 100 * time.Millisecond,
				Wait:          true,
				Fetch: func(_ context.Context, _ uuid.UUID) (codersdk.WorkspaceAgent, error) {
					if len(fetchSteps) == 0 {
						return agent, nil
					}
					step := fetchSteps[0]
					fetchSteps = fetchSteps[1:]
					step()
					return agent, nil
				},
				FetchLogs: func(_ context.Context, _ uuid.UUID, _ int64, follow bool) (<-chan []codersdk.WorkspaceAgentLog, io.Closer, error) {
					logsC := make(chan []codersdk.WorkspaceAgentLog, len(logs))
					if follow {
						go func() {
							defer close(logsC)
							for _, log := range logs {
								logsC <- []codersdk.WorkspaceAgentLog{log}
								time.Sleep(144 * time.Millisecond)
							}
							agent.LifecycleState = codersdk.WorkspaceAgentLifecycleReady
							readyAt := dbtime.Now()
							agent.ReadyAt = &readyAt
						}()
					} else {
						logsC <- logs
						close(logsC)
					}
					return logsC, closeFunc(func() error {
						return nil
					}), nil
				},
			})
			if err != nil {
				return err
			}
			return nil
		},
	})
	root.Children = append(root.Children, &serpent.Command{
		Use: "resources",
		Handler: func(inv *serpent.Invocation) error {
			disconnected := dbtime.Now().Add(-4 * time.Second)
			return cliui.WorkspaceResources(inv.Stdout, []codersdk.WorkspaceResource{{
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
				}, {
					DisconnectedAt:  &disconnected,
					Status:          codersdk.WorkspaceAgentDisconnected,
					LifecycleState:  codersdk.WorkspaceAgentLifecycleReady,
					Name:            "postgres",
					Architecture:    "amd64",
					OperatingSystem: "linux",
				}},
			}}, cliui.WorkspaceResourcesOptions{
				WorkspaceName:  "dev",
				HideAgentState: false,
				HideAccess:     false,
			})
		},
	})
	root.Children = append(root.Children, &serpent.Command{
		Use: "git-auth",
		Handler: func(inv *serpent.Invocation) error {
			var count atomic.Int32
			var githubAuthed atomic.Bool
			var gitlabAuthed atomic.Bool
			go func() {
				// Sleep to display the loading indicator.
				time.Sleep(time.Second)
				// Swap to true to display success and move onto GitLab.
				githubAuthed.Store(true)
				// Show the loading indicator again...
				time.Sleep(time.Second * 2)
				// Complete the auth!
				gitlabAuthed.Store(true)
			}()
			return cliui.ExternalAuth(inv.Context(), inv.Stdout, cliui.ExternalAuthOptions{
				Fetch: func(ctx context.Context) ([]codersdk.TemplateVersionExternalAuth, error) {
					count.Add(1)
					return []codersdk.TemplateVersionExternalAuth{{
						ID:              "github",
						Type:            codersdk.EnhancedExternalAuthProviderGitHub.String(),
						Authenticated:   githubAuthed.Load(),

						AuthenticateURL: "https://example.com/gitauth/github?redirect=" + url.QueryEscape("/gitauth?notify"),
					}, {
						ID:              "gitlab",
						Type:            codersdk.EnhancedExternalAuthProviderGitLab.String(),
						Authenticated:   gitlabAuthed.Load(),
						AuthenticateURL: "https://example.com/gitauth/gitlab?redirect=" + url.QueryEscape("/gitauth?notify"),
					}}, nil
				},
			})
		},
	})
	err := root.Invoke(os.Args[1:]...).WithOS().Run()
	if err != nil {
		_, _ = fmt.Println(err.Error())
		os.Exit(1)
	}
}
type closeFunc func() error
func (f closeFunc) Close() error {
	return f()
}
