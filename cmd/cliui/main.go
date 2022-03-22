package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
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
			_, err := cliui.Select(cmd, cliui.SelectOptions{
				Options: []string{"Tomato", "Banana", "Onion", "Grape", "Lemon"},
				Size:    3,
			})
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

			_, err := cliui.Job(cmd, cliui.JobOptions{
				Fetch: func() (codersdk.ProvisionerJob, error) {
					return job, nil
				},
				Logs: func() (<-chan codersdk.ProvisionerJobLog, error) {
					logs := make(chan codersdk.ProvisionerJobLog)
					go func() {
						defer close(logs)
						ticker := time.NewTicker(500 * time.Millisecond)
						for {
							select {
							case <-cmd.Context().Done():
								return
							case <-ticker.C:
								logs <- codersdk.ProvisionerJobLog{
									Output: "Some log",
									Level:  database.LogLevelInfo,
								}
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

	err := root.Execute()
	if err != nil {
		_, _ = fmt.Println(err.Error())
		os.Exit(1)
	}
}
