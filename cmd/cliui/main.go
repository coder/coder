package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func main() {
	root := &cobra.Command{
		Use:   "cliui",
		Short: "Used for visually testing UI components for the CLI.",
	}

	root.AddCommand(&cobra.Command{
		Use: "prompt",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cliui.Prompt(cmd, cliui.PromptOptions{
				Text:    "What is our " + cliui.Styles.Field.Render("company name") + "?",
				Default: "acme-corp",
				Validate: func(s string) error {
					if !strings.EqualFold(s, "coder") {
						return xerrors.New("Err... nope!")
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
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Do you want to accept?",
				Default:   "yes",
				IsConfirm: true,
			})
			if errors.Is(err, cliui.Canceled) {
				return nil
			}
			if err != nil {
				return err
			}
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:   "Enter password",
				Secret: true,
			})
			return err
		},
	})

	root.AddCommand(&cobra.Command{
		Use: "select",
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := cliui.Select(cmd, cliui.SelectOptions{
				Options: []string{"Tomato", "Banana", "Onion", "Grape", "Lemon"},
				Size:    3,
			})
			fmt.Printf("Selected: %q\n", value)
			return err
		},
	})

	root.AddCommand(&cobra.Command{
		Use: "job",
		RunE: func(cmd *cobra.Command, args []string) error {
			job := codersdk.ProvisionerJob{
				Status:    codersdk.ProvisionerJobPending,
				CreatedAt: database.Now(),
			}
			go func() {
				time.Sleep(time.Second)
				if job.Status != codersdk.ProvisionerJobPending {
					return
				}
				started := database.Now()
				job.StartedAt = &started
				job.Status = codersdk.ProvisionerJobRunning
				time.Sleep(3 * time.Second)
				if job.Status != codersdk.ProvisionerJobRunning {
					return
				}
				completed := database.Now()
				job.CompletedAt = &completed
				job.Status = codersdk.ProvisionerJobSucceeded
			}()

			err := cliui.ProvisionerJob(cmd.Context(), cmd.OutOrStdout(), cliui.ProvisionerJobOptions{
				Fetch: func() (codersdk.ProvisionerJob, error) {
					return job, nil
				},
				Logs: func() (<-chan codersdk.ProvisionerJobLog, error) {
					logs := make(chan codersdk.ProvisionerJobLog)
					go func() {
						defer close(logs)
						ticker := time.NewTicker(100 * time.Millisecond)
						defer ticker.Stop()
						count := 0
						for {
							select {
							case <-cmd.Context().Done():
								return
							case <-ticker.C:
								if job.Status == codersdk.ProvisionerJobSucceeded || job.Status == codersdk.ProvisionerJobCanceled {
									return
								}
								log := codersdk.ProvisionerJobLog{
									CreatedAt: time.Now(),
									Output:    fmt.Sprintf("Some log %d", count),
									Level:     database.LogLevelInfo,
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
									log.Level = database.LogLevelWarn
								}
								count++
								if log.Output == "" && log.Stage == "" {
									continue
								}
								logs <- log
							}
						}
					}()
					return logs, nil
				},
				Cancel: func() error {
					job.Status = codersdk.ProvisionerJobCanceling
					time.Sleep(time.Second)
					job.Status = codersdk.ProvisionerJobCanceled
					completed := database.Now()
					job.CompletedAt = &completed
					return nil
				},
			})
			return err
		},
	})

	root.AddCommand(&cobra.Command{
		Use: "agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			resource := codersdk.WorkspaceResource{
				Type: "google_compute_instance",
				Name: "dev",
				Agent: &codersdk.WorkspaceAgent{
					Status: codersdk.WorkspaceAgentDisconnected,
				},
			}
			go func() {
				time.Sleep(3 * time.Second)
				resource.Agent.Status = codersdk.WorkspaceAgentConnected
			}()
			err := cliui.Agent(cmd.Context(), cmd.OutOrStdout(), cliui.AgentOptions{
				WorkspaceName: "dev",
				Fetch: func(ctx context.Context) (codersdk.WorkspaceResource, error) {
					return resource, nil
				},
				WarnInterval: 2 * time.Second,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Completed!\n")
			return nil
		},
	})

	err := root.Execute()
	if err != nil {
		_, _ = fmt.Println(err.Error())
		os.Exit(1)
	}
}
