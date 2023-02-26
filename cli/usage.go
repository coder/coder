package cli

import (
	_ "embed"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/mitchellh/go-wordwrap"

	"github.com/coder/coder/cli/bigcli"
)

//go:embed usage.tpl
var usageTemplateRaw string

type optionGroup struct {
	Name        string
	Description string
	Options     bigcli.OptionSet
}

var optionGroupDescriptions = map[string]string{
	"Networking": `
Configure how coder server connects to users and workspaces.
`,
}

const envPrefix = "CODER_"

var usageTemplate = template.Must(
	template.New("usage").Funcs(
		template.FuncMap{
			"wordWrap": func(s string, width uint) string {
				return wordwrap.WrapString(s, width)
			},
			"indent": func(s string, tabs int) string {
				var sb strings.Builder
				for _, line := range strings.Split(s, "\n") {
					// Remove existing indent, if any.
					line = strings.TrimSpace(line)
					_, _ = sb.WriteString(strings.Repeat("\t", tabs))
					_, _ = sb.WriteString(line)
					_, _ = sb.WriteString("\n")
				}
				return sb.String()
			},
			"envName": func(opt bigcli.Option) string {
				n, ok := opt.EnvName()
				if !ok {
					return ""
				}
				return envPrefix + n
			},
			"flagName": func(opt bigcli.Option) string {
				n, _ := opt.FlagName()
				return n
			},
			"optionGroups": func(cmd *bigcli.Command) []optionGroup {
				groups := []optionGroup{{
					// Default group.
					Name:        "",
					Description: "",
				}}

			optionLoop:
				for _, opt := range cmd.Options {
					if opt.Hidden {
						continue
					}
					groupName, ok := opt.Annotations.Get("group")
					if !ok {
						// Just add option to default group.
						groups[0].Options = append(groups[0].Options, opt)
						continue
					}

					for i, foundGroup := range groups {
						if foundGroup.Name != groupName {
							continue
						}
						groups[i].Options = append(groups[i].Options, opt)
						continue optionLoop
					}

					groups = append(groups, optionGroup{
						Name:        groupName,
						Description: optionGroupDescriptions[groupName],
						Options:     bigcli.OptionSet{opt},
					})
				}
				return groups
			},
		},
	).Parse(usageTemplateRaw))

// usageFn returns a function that generates usage (help)
// output for a given command.
func usageFn(output io.Writer, cmd *bigcli.Command) func() {
	return func() {
		err := usageTemplate.Execute(output, cmd)
		if err != nil {
			_, _ = fmt.Fprintf(output, "execute template: %v", err)
		}
	}
}
