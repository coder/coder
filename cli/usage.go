package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/creack/pty"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
)

var templateFunctions = template.FuncMap{
	"usageHeader":        usageHeader,
	"isWorkspaceCommand": isWorkspaceCommand,
	"ttyWidth":           ttyWidth,
	"categorizeFlags":    categorizeFlags,
}

func usageHeader(s string) string {
	// Customizes the color of headings to make subcommands more visually
	// appealing.
	return cliui.Styles.Placeholder.Render(s)
}

func isWorkspaceCommand(cmd *cobra.Command) bool {
	if _, ok := cmd.Annotations["workspaces"]; ok {
		return true
	}
	var ws bool
	cmd.VisitParents(func(cmd *cobra.Command) {
		if _, ok := cmd.Annotations["workspaces"]; ok {
			ws = true
		}
	})
	return ws
}
func ttyWidth() int {
	_, cols, err := pty.Getsize(os.Stderr)
	if err != nil {
		// Default width
		return 100
	}
	return cols
}

type flagCategory struct {
	name     string
	matchers []*regexp.Regexp
}

// flagCategories are evaluated by categorizeFlags in order. The first matched
// category is used for each flag declaration.
var flagCategories = []flagCategory{
	{
		name: "Networking",
		matchers: []*regexp.Regexp{
			regexp.MustCompile("derp"),
			regexp.MustCompile("access-url"),
			regexp.MustCompile("http-address"),
			regexp.MustCompile("proxy"),
			regexp.MustCompile("auth-cookie"),
			regexp.MustCompile("strict-transport"),
			regexp.MustCompile("tls"),
			regexp.MustCompile("telemetry"),
			regexp.MustCompile("update-check"),
		},
	},
	{
		name: "Auth",
		matchers: []*regexp.Regexp{
			regexp.MustCompile("oauth2"),
			regexp.MustCompile("oidc"),
			regexp.MustCompile(`-\w*token`),
			regexp.MustCompile("session"),
		},
	},
	{
		name: "Operability",
		matchers: []*regexp.Regexp{
			regexp.MustCompile("--log"),
			regexp.MustCompile("pprof"),
			regexp.MustCompile("prometheus"),
			regexp.MustCompile("trace"),
		},
	},
	{
		name: "Provisioning",
		matchers: []*regexp.Regexp{
			regexp.MustCompile("--provisioner"),
		},
	},
	{
		name: "Other",
		matchers: []*regexp.Regexp{
			// Everything!
			regexp.MustCompile("."),
		},
	},
}

// categorizeFlags makes the `coder server --help` output bearable by grouping
// similar flags. This approach is janky, but the alternative is reimplementing
// https://github.com/spf13/pflag/blob/v1.0.5/flag.go#L677, which involves
// hundreds of lines of complex code since key functions are internal.
func categorizeFlags(usageOutput string) string {
	var out strings.Builder

	var (
		sc          = bufio.NewScanner(strings.NewReader(usageOutput))
		currentFlag bytes.Buffer
		categories  = make(map[string]*bytes.Buffer)
	)
	flushCurrentFlag := func() {
		if currentFlag.Len() == 0 {
			return
		}

		for _, cat := range flagCategories {
			for _, matcher := range cat.matchers {
				if matcher.MatchString(currentFlag.String()) {
					if _, ok := categories[cat.name]; !ok {
						categories[cat.name] = &bytes.Buffer{}
					}
					_, _ = categories[cat.name].WriteString(currentFlag.String())
					currentFlag.Reset()
					return
				}
			}
		}

		_, _ = out.WriteString("ERROR: no category matched for flag")
		_, _ = currentFlag.WriteTo(&out)
	}
	for sc.Scan() {
		if strings.HasPrefix(strings.TrimSpace(sc.Text()), "-") {
			// Beginning of a new flag, flush old.
			flushCurrentFlag()
		}
		_, _ = currentFlag.WriteString(sc.Text())
		_ = currentFlag.WriteByte('\n')
	}
	flushCurrentFlag()

	for _, cat := range flagCategories {
		if buf, ok := categories[cat.name]; ok {
			if len(categories) == 1 {
				// Don't bother qualifying list if there's only one category.
				_, _ = fmt.Fprintf(&out, "%s\n", usageHeader("Flags:"))
			} else {
				_, _ = fmt.Fprintf(&out, "%s\n", usageHeader(cat.name+" Flags:"))
			}
			_, _ = buf.WriteTo(&out)
		}
	}

	return out.String()
}

func usageTemplate() string {
	// usageHeader is defined in init().
	return `{{usageHeader "Usage:"}}
{{- if .Runnable}}
  {{.UseLine}}
{{end}}
{{- if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]
{{end}}

{{- if gt (len .Aliases) 0}}
{{usageHeader "Aliases:"}}
  {{.NameAndAliases}}
{{end}}

{{- if .HasExample}}
{{usageHeader "Get Started:"}}
{{.Example}}
{{end}}

{{- $isRootHelp := (not .HasParent)}}
{{- if .HasAvailableSubCommands}}
{{usageHeader "Commands:"}}
  {{- range .Commands}}
    {{- $isRootWorkspaceCommand := (and $isRootHelp (isWorkspaceCommand .))}}
    {{- if (or (and .IsAvailableCommand (not $isRootWorkspaceCommand)) (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}
    {{- end}}
  {{- end}}
{{end}}

{{- if (and $isRootHelp .HasAvailableSubCommands)}}
{{usageHeader "Workspace Commands:"}}
  {{- range .Commands}}
    {{- if (and .IsAvailableCommand (isWorkspaceCommand .))}}
  {{rpad .Name .NamePadding }} {{.Short}}
    {{- end}}
  {{- end}}
{{end}}

{{- if .HasAvailableLocalFlags}}
{{.LocalFlags.FlagUsagesWrapped ttyWidth | categorizeFlags | trimTrailingWhitespaces}}
{{end}}

{{- if .HasAvailableInheritedFlags}}
{{usageHeader "Global Flags:"}}
{{.InheritedFlags.FlagUsagesWrapped ttyWidth | trimTrailingWhitespaces}}
{{end}}

{{- if .HasHelpSubCommands}}
{{usageHeader "Additional help topics:"}}
  {{- range .Commands}}
    {{- if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}
    {{- end}}
  {{- end}}
{{end}}

{{- if .HasAvailableSubCommands}}
Use "{{.CommandPath}} [command] --help" for more information about a command.
{{end}}`
}

// example represents a standard example for command usage, to be used
// with formatExamples.
type example struct {
	Description string
	Command     string
}

// formatExamples formats the examples as width wrapped bulletpoint
// descriptions with the command underneath.
func formatExamples(examples ...example) string {
	wrap := cliui.Styles.Wrap.Copy()
	wrap.PaddingLeft(4)
	var sb strings.Builder
	for i, e := range examples {
		if len(e.Description) > 0 {
			_, _ = sb.WriteString("  - " + wrap.Render(e.Description + ":")[4:] + "\n\n    ")
		}
		// We add 1 space here because `cliui.Styles.Code` adds an extra
		// space. This makes the code block align at an even 2 or 6
		// spaces for symmetry.
		_, _ = sb.WriteString(" " + cliui.Styles.Code.Render(fmt.Sprintf("$ %s", e.Command)))
		if i < len(examples)-1 {
			_, _ = sb.WriteString("\n\n")
		}
	}
	return sb.String()
}
