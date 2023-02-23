package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/acarl005/stripansi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	_ "embed"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/flog"
)

//go:embed command.tpl
var commandTemplateRaw string

var commandTemplate *template.Template

var envRegex = regexp.MustCompile(`Consumes (\$\w+).?$`)

func parseEnv(flagUsage string) string {
	flagUsage = stripansi.Strip(flagUsage)

	ss := envRegex.FindStringSubmatch(flagUsage)
	if len(ss) == 0 {
		return ""
	}
	return ss[len(ss)-1]
}

func stripEnv(flagUsage string) string {
	flagUsage = stripansi.Strip(flagUsage)
	ss := envRegex.FindStringSubmatch(flagUsage)
	if len(ss) == 0 {
		return flagUsage
	}
	return strings.TrimSpace(strings.ReplaceAll(flagUsage, ss[0], ""))
}

func init() {
	commandTemplate = template.Must(
		template.New("command.tpl").Funcs(template.FuncMap{
			"newLinesToBr": func(s string) string {
				return strings.ReplaceAll(s, "\n", "<br/>")
			},
			"wrapCode": func(s string) string {
				return fmt.Sprintf("<code>%s</code>", s)
			},
			"parseEnv": parseEnv,
			"stripEnv": stripEnv,
			"commandURI": func(cmd *cobra.Command) string {
				return strings.TrimSuffix(
					fmtDocFilename(cmd),
					".md",
				)
			},
		},
		).Parse(strings.TrimSpace(commandTemplateRaw)),
	)
}

func writeCommand(w io.Writer, cmd *cobra.Command) error {
	var (
		flags          []*pflag.Flag
		inheritedFlags []*pflag.Flag
	)
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flags = append(flags, f)
	})
	cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		inheritedFlags = append(inheritedFlags, f)
	})
	var b strings.Builder
	err := commandTemplate.Execute(&b, map[string]any{
		"Name":           fullCommandName(cmd),
		"Cmd":            cmd,
		"Flags":          flags,
		"InheritedFlags": inheritedFlags,
		"AtRoot":         cmd.Parent() == nil,
		"VisibleSubcommands": func() []*cobra.Command {
			var scs []*cobra.Command
			for _, sub := range cmd.Commands() {
				if sub.Hidden {
					continue
				}
				scs = append(scs, sub)
			}
			return scs
		}(),
	})
	if err != nil {
		return err
	}
	content := stripansi.Strip(b.String())

	// Remove the version and its right space, since during this script running
	// there is no build info available
	content = strings.ReplaceAll(content, buildinfo.Version()+" ", "")

	// Remove references to the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, cwd, ".")

	_, err = w.Write([]byte(content))
	return err
}

func fullCommandName(cmd *cobra.Command) string {
	name := cmd.Name()
	if cmd.Parent() != nil {
		return fullCommandName(cmd.Parent()) + " " + name
	}
	return name
}

func fmtDocFilename(cmd *cobra.Command) string {
	fullName := fullCommandName(cmd)
	if fullName == "coder" {
		// Special case for index.
		return "../cli.md"
	}
	name := strings.ReplaceAll(fullName, " ", "_")
	return fmt.Sprintf("%s.md", name)
}

func generateDocsTree(rootCmd *cobra.Command, basePath string) error {
	if rootCmd.Hidden {
		return nil
	}

	// Write out root.
	fi, err := os.OpenFile(
		filepath.Join(basePath, fmtDocFilename(rootCmd)),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644,
	)
	if err != nil {
		return err
	}
	defer fi.Close()

	err = writeCommand(fi, rootCmd)
	if err != nil {
		return err
	}

	flog.Info("Generated docs for %q at %v", fullCommandName(rootCmd), fi.Name())

	// Recursively generate docs.
	for _, subcommand := range rootCmd.Commands() {
		err = generateDocsTree(subcommand, basePath)
		if err != nil {
			return err
		}
	}
	return nil
}
