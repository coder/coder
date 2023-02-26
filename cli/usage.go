package cli

import (
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"

	"github.com/mitchellh/go-wordwrap"

	"github.com/coder/coder/cli/bigcli"
	"github.com/coder/coder/cli/cliui"
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
Configure TLS, the wildcard access URL, bind addresses, access URLs, etc.
`,
	"Networking / DERP": `
Most Coder deployments never have to think about DERP because all connections
between workspaces and users are peer-to-peer. However, when Coder cannot establish
a peer to peer connection, Coder uses a distributed relay network backed by
Tailscale and WireGuard.
`,
	"Networking / TLS": `
Configure TLS / HTTPS for your Coder deployment. If you're running
Coder behind a TLS-terminating reverse proxy or are accessing Coder over a
secure link, you can safely ignore these settings. 
`,
	`Introspection`: `
Configure logging, tracing, and metrics exporting.
`,
	`oAuth2`: `
Configure login and user-provisioning with GitHub via oAuth2.
`,
	`OIDC`: `
Configure login and user-provisioning with OIDC.
`,
	`Telemetry`: `
Telemetry is critical to our ability to improve Coder. We strip all personal
information before sending data to our servers. Please only disable telemetry
when required by your organization's security policy.
`,
	`Provisioning`: `
Tune the behavior of the provisioner, which is responsible for creating,
updating, and deleting workspace resources.
`,
	`Config`: `
Use a YAML configuration file when your server launch become unwieldy.
`,
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
			"prettyHeader": func(s string) string {
				return cliui.Styles.Bold.Render(s)
			},
			"isEnterprise": func(opt bigcli.Option) bool {
				return opt.Annotations.IsSet("enterprise")
			},
			"isDeprecated": func(opt bigcli.Option) bool {
				return len(opt.UseInstead) > 0
			},
			"formatDescription": func(s string) string {
				s = "\n" + s + "\n"
				s = wordwrap.WrapString(s, 60)
				return s
			},
			"optionGroups": func(cmd *bigcli.Command) []optionGroup {
				groups := []optionGroup{{
					// Default group.
					Name:        "",
					Description: "",
				}}

				enterpriseGroup := optionGroup{
					Name: "Enterprise",
					Description: `
These options are only available in the Enterprise Edition.
`,
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
					if len(opt.Group) == 0 {
						// Just add option to default group.
						groups[0].Options = append(groups[0].Options, opt)
						continue
					}

					groupName := strings.Join(opt.Group, " / ")

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
func usageFn(output io.Writer, cmd *bigcli.Command) func() {
	return func() {
		err := usageTemplate.Execute(output, cmd)
		if err != nil {
			_, _ = fmt.Fprintf(output, "execute template: %v", err)
		}
	}
}
