package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/coder/coder/cli/cliui"
	"github.com/spf13/cobra"
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
				Prompt:  "What is our " + cliui.Styles.Field.Render("company name") + "?",
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
			fmt.Printf("You got it!\n")
			return nil
		},
	})

	err := root.Execute()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
