package cli

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (RootCmd) promptExample() *serpent.Command {
	promptCmd := func(use string, prompt func(inv *serpent.Invocation) error, options ...serpent.Option) *serpent.Command {
		return &serpent.Command{
			Use:     use,
			Options: options,
			Handler: func(inv *serpent.Invocation) error {
				return prompt(inv)
			},
		}
	}

	var useSearch bool
	useSearchOption := serpent.Option{
		Name:        "search",
		Description: "Show the search.",
		Required:    false,
		Flag:        "search",
		Value:       serpent.BoolOf(&useSearch),
	}
	cmd := &serpent.Command{
		Use:   "prompt-example",
		Short: "Example of various prompt types used within coder cli.",
		Long: "Example of various prompt types used within coder cli. " +
			"This command exists to aid in adjusting visuals of command prompts.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			promptCmd("confirm", func(inv *serpent.Invocation) error {
				value, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "Basic confirmation prompt.",
					Default:   "yes",
					IsConfirm: true,
				})
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", value)
				return err
			}),
			promptCmd("validation", func(inv *serpent.Invocation) error {
				value, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "Input a string that starts with a capital letter.",
					Default:   "",
					Secret:    false,
					IsConfirm: false,
					Validate: func(s string) error {
						if len(s) == 0 {
							return xerrors.Errorf("an input string is required")
						}
						if strings.ToUpper(string(s[0])) != string(s[0]) {
							return xerrors.Errorf("input string must start with a capital letter")
						}
						return nil
					},
				})
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", value)
				return err
			}),
			promptCmd("secret", func(inv *serpent.Invocation) error {
				value, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "Input a secret",
					Default:   "",
					Secret:    true,
					IsConfirm: false,
					Validate: func(s string) error {
						if len(s) == 0 {
							return xerrors.Errorf("an input string is required")
						}
						return nil
					},
				})
				_, _ = fmt.Fprintf(inv.Stdout, "Your secret of length %d is safe with me\n", len(value))
				return err
			}),
			promptCmd("select", func(inv *serpent.Invocation) error {
				value, err := cliui.Select(inv, cliui.SelectOptions{
					Options: []string{
						"Blue", "Green", "Yellow", "Red", "Something else",
					},
					Default:    "",
					Message:    "Select your favorite color:",
					Size:       5,
					HideSearch: !useSearch,
				})
				if value == "Something else" {
					_, _ = fmt.Fprint(inv.Stdout, "I would have picked blue.\n")
				} else {
					_, _ = fmt.Fprintf(inv.Stdout, "%s is a nice color.\n", value)
				}
				return err
			}, useSearchOption),
			promptCmd("multiple", func(inv *serpent.Invocation) error {
				_, _ = fmt.Fprintf(inv.Stdout, "This command exists to test the behavior of multiple prompts. The survey library does not erase the original message prompt after.")
				thing, err := cliui.Select(inv, cliui.SelectOptions{
					Message: "Select a thing",
					Options: []string{
						"Car", "Bike", "Plane", "Boat", "Train",
					},
					Default: "Car",
				})
				if err != nil {
					return err
				}
				color, err := cliui.Select(inv, cliui.SelectOptions{
					Message: "Select a color",
					Options: []string{
						"Blue", "Green", "Yellow", "Red",
					},
					Default: "Blue",
				})
				if err != nil {
					return err
				}
				properties, err := cliui.MultiSelect(inv, cliui.MultiSelectOptions{
					Message: "Select properties",
					Options: []string{
						"Fast", "Cool", "Expensive", "New",
					},
					Defaults: []string{"Fast"},
				})
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(inv.Stdout, "Your %s %s is awesome! Did you paint it %s?\n",
					strings.Join(properties, " "),
					thing,
					color,
				)
				return err
			}),
			promptCmd("multi-select", func(inv *serpent.Invocation) error {
				values, err := cliui.MultiSelect(inv, cliui.MultiSelectOptions{
					Message: "Select some things:",
					Options: []string{
						"Code", "Chair", "Whale", "Diamond", "Carrot",
					},
					Defaults: []string{"Code"},
				})
				_, _ = fmt.Fprintf(inv.Stdout, "%q are nice choices.\n", strings.Join(values, ", "))
				return err
			}),
			promptCmd("rich-parameter", func(inv *serpent.Invocation) error {
				value, err := cliui.RichSelect(inv, cliui.RichSelectOptions{
					Options: []codersdk.TemplateVersionParameterOption{
						{
							Name:        "Blue",
							Description: "Like the ocean.",
							Value:       "blue",
							Icon:        "/logo/blue.png",
						},
						{
							Name:        "Red",
							Description: "Like a clown's nose.",
							Value:       "red",
							Icon:        "/logo/red.png",
						},
						{
							Name:        "Yellow",
							Description: "Like a bumblebee. ",
							Value:       "yellow",
							Icon:        "/logo/yellow.png",
						},
					},
					Default:    "blue",
					Size:       5,
					HideSearch: useSearch,
				})
				_, _ = fmt.Fprintf(inv.Stdout, "%s is a good choice.\n", value.Name)
				return err
			}, useSearchOption),
		},
	}

	return cmd
}
