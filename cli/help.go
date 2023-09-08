package cli

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"text/template"
	"unicode"

	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
)

//go:embed help.tpl
var helpTemplateRaw string

type optionGroup struct {
	Name        string
	Description string
	Options     clibase.OptionSet
}

func ttyWidth() int {
	width, _, err := terminal.GetSize(0)
	if err != nil {
		return 80
	}
	return width
}

// wrapTTY wraps a string to the width of the terminal, or 80 no terminal
// is detected.
func wrapTTY(s string) string {
	return wordwrap.WrapString(s, uint(ttyWidth()))
}

var usageTemplate = template.Must(
	template.New("usage").Funcs(
		template.FuncMap{
			"version": func() string {
				return buildinfo.Version()
			},
			"wrapTTY": func(s string) string {
				return wrapTTY(s)
			},
			"trimNewline": func(s string) string {
				return strings.TrimSuffix(s, "\n")
			},
			"keyword": func(s string) string {
				return pretty.Sprint(
					pretty.FgColor(cliui.Color("#87ceeb")),
					s,
				)
			},
			"prettyHeader": func(s string) string {
				return pretty.Sprint(
					pretty.FgColor(
						cliui.Color("#ffb500"),
					), strings.ToUpper(s), ":",
				)
			},
			"typeHelper": func(opt *clibase.Option) string {
				switch v := opt.Value.(type) {
				case *clibase.Enum:
					return strings.Join(v.Choices, "|")
				default:
					return v.Type()
				}
			},
			"joinStrings": func(s []string) string {
				return strings.Join(s, ", ")
			},
			"indent": func(body string, spaces int) string {
				twidth := ttyWidth()

				spacing := strings.Repeat(" ", spaces)

				body = wordwrap.WrapString(body, uint(twidth-len(spacing)))

				sc := bufio.NewScanner(strings.NewReader(body))

				var sb strings.Builder
				for sc.Scan() {
					// Remove existing indent, if any.
					// line = strings.TrimSpace(line)
					// Use spaces so we can easily calculate wrapping.
					_, _ = sb.WriteString(spacing)
					_, _ = sb.Write(sc.Bytes())
					_, _ = sb.WriteString("\n")
				}
				return sb.String()
			},
			"formatSubcommand": func(cmd *clibase.Cmd) string {
				// Minimize padding by finding the longest neighboring name.
				maxNameLength := len(cmd.Name())
				if parent := cmd.Parent; parent != nil {
					for _, c := range parent.Children {
						if len(c.Name()) > maxNameLength {
							maxNameLength = len(c.Name())
						}
					}
				}

				var sb strings.Builder
				_, _ = fmt.Fprintf(
					&sb, "%s%s%s",
					strings.Repeat(" ", 4), cmd.Name(), strings.Repeat(" ", maxNameLength-len(cmd.Name())+4),
				)

				// This is the point at which indentation begins if there's a
				// next line.
				descStart := sb.Len()

				twidth := ttyWidth()

				for i, line := range strings.Split(
					wordwrap.WrapString(cmd.Short, uint(twidth-descStart)), "\n",
				) {
					if i > 0 {
						_, _ = sb.WriteString(strings.Repeat(" ", descStart))
					}
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

			"isEnterprise": func(opt clibase.Option) bool {
				return opt.Annotations.IsSet("enterprise")
			},
			"isDeprecated": func(opt clibase.Option) bool {
				return len(opt.UseInstead) > 0
			},
			"useInstead": func(opt clibase.Option) string {
				var sb strings.Builder
				for i, s := range opt.UseInstead {
					if i > 0 {
						if i == len(opt.UseInstead)-1 {
							_, _ = sb.WriteString(" and ")
						} else {
							_, _ = sb.WriteString(", ")
						}
					}
					if s.Flag != "" {
						_, _ = sb.WriteString("--")
						_, _ = sb.WriteString(s.Flag)
					} else if s.FlagShorthand != "" {
						_, _ = sb.WriteString("-")
						_, _ = sb.WriteString(s.FlagShorthand)
					} else if s.Env != "" {
						_, _ = sb.WriteString("$")
						_, _ = sb.WriteString(s.Env)
					} else {
						_, _ = sb.WriteString(s.Name)
					}
				}
				return sb.String()
			},
			"formatGroupDescription": func(s string) string {
				s = strings.ReplaceAll(s, "\n", "")
				s = s + "\n"
				s = wrapTTY(s)
				return s
			},
			"visibleChildren": func(cmd *clibase.Cmd) []*clibase.Cmd {
				return filterSlice(cmd.Children, func(c *clibase.Cmd) bool {
					return !c.Hidden
				})
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

// newLineLimiter makes working with Go templates more bearable. Without this,
// modifying the template is a slow toil of counting newlines and constantly
// checking that a change to one command's help doesn't clobber break another.
type newlineLimiter struct {
	w     io.Writer
	limit int

	newLineCounter int
}

func (lm *newlineLimiter) Write(p []byte) (int, error) {
	rd := bytes.NewReader(p)
	for r, n, _ := rd.ReadRune(); n > 0; r, n, _ = rd.ReadRune() {
		switch {
		case r == '\r':
			// Carriage returns can sneak into `help.tpl` when `git clone`
			// is configured to automatically convert line endings.
			continue
		case r == '\n':
			lm.newLineCounter++
			if lm.newLineCounter > lm.limit {
				continue
			}
		case !unicode.IsSpace(r):
			lm.newLineCounter = 0
		}
		_, err := lm.w.Write([]byte(string(r)))
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

var usageWantsArgRe = regexp.MustCompile(`<.*>`)

// helpFn returns a function that generates usage (help)
// output for a given command.
func helpFn() clibase.HandlerFunc {
	return func(inv *clibase.Invocation) error {
		// We use stdout for help and not stderr since there's no straightforward
		// way to distinguish between a user error and a help request.
		//
		// We buffer writes to stdout because the newlineLimiter writes one
		// rune at a time.
		outBuf := bufio.NewWriter(inv.Stdout)
		out := newlineLimiter{w: outBuf, limit: 2}
		tabwriter := tabwriter.NewWriter(&out, 0, 0, 2, ' ', 0)
		err := usageTemplate.Execute(tabwriter, inv.Command)
		if err != nil {
			return xerrors.Errorf("execute template: %w", err)
		}
		err = tabwriter.Flush()
		if err != nil {
			return err
		}
		err = outBuf.Flush()
		if err != nil {
			return err
		}
		if len(inv.Args) > 0 && !usageWantsArgRe.MatchString(inv.Command.Use) {
			_, _ = fmt.Fprintf(inv.Stderr, "---\nerror: unknown subcommand %q\n", inv.Args[0])
		}
		return nil
	}
}
