package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
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
				Text:    "Pick an option!",
				Options: []string{"Tomato", "Banana", "Onion", "Grape", "Lemon"},
				Size:    3,
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
