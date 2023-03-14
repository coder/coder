package cli

import (
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"

	"github.com/mitchellh/go-wordwrap"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

//go:embed usage.tpl
var usageTemplateRaw string

type optionGroup struct {
	Name        string
	Description string
	Options     clibase.OptionSet
}

const envPrefix = "CODER_"

var usageTemplate = template.Must(
	template.New("usage").Funcs(
		template.FuncMap{
			"wordWrap": func(s string, width uint) string {
				return wordwrap.WrapString(s, width)
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
				return envPrefix + opt.Env
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
			"formatGroupDescription": func(s string) string {
				s = strings.ReplaceAll(s, "\n", "")
				s = "\n" + s + "\n"
				s = wordwrap.WrapString(s, 60)
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
				return append(groups, enterpriseGroup)
			},
		},
	).Parse(usageTemplateRaw),
)

// usageFn returns a function that generates usage (help)
// output for a given command.
func usageFn(output io.Writer, cmd *clibase.Cmd) func() {
	return func() {
		err := usageTemplate.Execute(output, cmd)
		if err != nil {
			_, _ = fmt.Fprintf(output, "execute template: %v", err)
		}
	}
}
