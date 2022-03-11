package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
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

	err := root.Execute()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
