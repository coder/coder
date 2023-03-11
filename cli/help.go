package cli

import (
	_ "embed"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

//go:embed help.tpl
var helpTemplateRaw string

type optionGroup struct {
	Name        string
	Description string
	Options     clibase.OptionSet
}

// wrapTTY wraps a string to the width of the terminal, or 80 no terminal
// is detected.
func wrapTTY(s string) string {
	width, _, err := terminal.GetSize(0)
	if err != nil {
		width = 80
	}
	return wordwrap.WrapString(s, uint(width))
}

var usageTemplate = template.Must(
	template.New("usage").Funcs(
		template.FuncMap{
			"wrapTTY": func(s string) string {
				return wrapTTY(s)
			},
			"trimNewline": func(s string) string {
				return strings.TrimSuffix(s, "\n")
			},
			"indent": func(s string, tabs int) string {
				var sb strings.Builder
				for _, line := range strings.Split(s, "\n") {
					// Remove existing indent, if any.
					_, _ = sb.WriteString(strings.Repeat("\t", tabs))
					_, _ = sb.WriteString(line)
					_, _ = sb.WriteString("\n")
				}
				return sb.String()
			},
			"envName": func(opt clibase.Option) string {
				if opt.Env == "" {
					return ""
				}
				return opt.Env
			},
			"flagName": func(opt clibase.Option) string {
				return opt.Flag
			},
			"prettyHeader": func(s string) string {
				return cliui.Styles.Bold.Render(s)
			},
			"isEnterprise": func(opt clibase.Option) bool {
				return opt.Annotations.IsSet("enterprise")
			},
			"isDeprecated": func(opt clibase.Option) bool {
				return len(opt.UseInstead) > 0
			},
			"formatLong": func(long string) string {
				return wrapTTY(strings.TrimSpace(long))
			},
			"formatGroupDescription": func(s string) string {
				s = strings.ReplaceAll(s, "\n", "")
				s = s + "\n"
				s = wrapTTY(s)
				return s
			},
			"optionGroups": func(cmd *clibase.Cmd) []optionGroup {
				groups := []optionGroup{{
					// Default group.
					Name:        "",
					Description: "",
				}}

				enterpriseGroup := optionGroup{
					Name:        "Enterprise",
					Description: `These options are only available in the Enterprise Edition.`,
				}

				// Sort options lexicographically.
				sort.Slice(cmd.Options, func(i, j int) bool {
					return cmd.Options[i].Name < cmd.Options[j].Name
				})

			optionLoop:
				for _, opt := range cmd.Options {
					if opt.Hidden {
						continue
					}
					// Enterprise options are always grouped separately.
					if opt.Annotations.IsSet("enterprise") {
						enterpriseGroup.Options = append(enterpriseGroup.Options, opt)
						continue
					}
					if len(opt.Group.Ancestry()) == 0 {
						// Just add option to default group.
						groups[0].Options = append(groups[0].Options, opt)
						continue
					}

					groupName := opt.Group.FullName()

					for i, foundGroup := range groups {
						if foundGroup.Name != groupName {
							continue
						}
						groups[i].Options = append(groups[i].Options, opt)
						continue optionLoop
					}

					groups = append(groups, optionGroup{
						Name:        groupName,
						Description: opt.Group.Description,
						Options:     clibase.OptionSet{opt},
					})
				}
				sort.Slice(groups, func(i, j int) bool {
					// Sort groups lexicographically.
					return groups[i].Name < groups[j].Name
				})

				// Always show enterprise group last.
				groups = append(groups, enterpriseGroup)

				return filterSlice(groups, func(g optionGroup) bool {
					return len(g.Options) > 0
				})
			},
		},
	).Parse(helpTemplateRaw),
)

func filterSlice[T any](s []T, f func(T) bool) []T {
	var r []T
	for _, v := range s {
		if f(v) {
			r = append(r, v)
		}
	}
	return r
}

// helpFn returns a function that generates usage (help)
// output for a given command.
func helpFn() clibase.HandlerFunc {
	return func(inv *clibase.Invocation) error {
		tabwriter := tabwriter.NewWriter(inv.Stderr, 0, 0, 2, ' ', 0)
		err := usageTemplate.Execute(tabwriter, inv.Command)
		if err != nil {
			return xerrors.Errorf("execute template: %w", err)
		}
		err = tabwriter.Flush()
		if err != nil {
			panic(err)
		}
		return nil
	}
}
