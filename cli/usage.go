package cli

import (
	_ "embed"
	"fmt"
	"io"
	"text/template"

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

var usageTemplate = template.Must(
	template.New("usage").Funcs(
		template.FuncMap{
			"optionGroups": func(cmd *bigcli.Command) []optionGroup {
				groups := []optionGroup{{
					// Default group.
					Name:        "",
					Description: "",
				}}

			optionLoop:
				for _, opt := range cmd.Options {
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
