package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

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
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliui.List(cmd, cliui.ListOptions{
				Items: []cliui.ListItem{{
					Title:       "Example",
					Description: "Something...",
				}, {
					Title:       "Wow, here's another!",
					Description: "Another exciting description!",
				}},
			})
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use: "prompt",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cliui.Prompt(cmd, cliui.PromptOptions{
				Text:    "What is our " + cliui.Styles.Field.Render("company name") + "?",
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
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use: "parameter",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cliui.Parameter(cmd, codersdk.ProjectVersionParameterSchema{
				Name:                     "region",
				ValidationCondition:      `contains(["us-east-1", "us-central-1"], var.region)`,
				ValidationTypeSystem:     database.ParameterTypeSystemHCL,
				RedisplayValue:           true,
				DefaultSourceScheme:      database.ParameterSourceSchemeData,
				DefaultSourceValue:       "us-east-1",
				AllowOverrideSource:      true,
				DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
				Description: `Specify a region for your workspace to live!
				https://cloud.google.com/compute/docs/regions-zones#available`,
			}, codersdk.ProjectVersionParameter{
				ParameterValue: database.ParameterValue{
					Scope:        database.ParameterScopeProject,
					ScopeID:      "something",
					SourceScheme: database.ParameterSourceSchemeData,
					SourceValue:  "",
				},
				DefaultSourceValue: true,
			})
		},
	})

	err := root.Execute()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
