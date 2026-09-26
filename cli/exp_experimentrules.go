package cli

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

const experimentRuleRevisionHelp = "Without --expected-revision, the command reads the current revision " +
	"and writes against it. That protects only against a change made between that read and the write. " +
	"A conflicting change fails the command; it is never retried automatically."

func (r *RootCmd) experimentRulesCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "experiment-rules",
		Short: "Manage runtime rules for user-scoped experiments",
		Long: "Runtime rules turn user-scoped experiments on or off, or target them with a CEL " +
			"condition over `user`, without a restart. The API and this command are experimental.\n" +
			FormatExamples(
				Example{
					Description: "Enable an experiment for users whose condition is true",
					Command:     `coder exp experiment-rules set mcp-tool-search 'user.email.endsWith("@example.com")'`,
				},
				Example{
					Description: "Turn an experiment off for everyone, even if it is enabled at startup",
					Command:     "coder exp experiment-rules off mcp-tool-search",
				},
				Example{
					Description: "Restore the startup --experiments default",
					Command:     "coder exp experiment-rules reset mcp-tool-search",
				},
			),
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.experimentRulesList(),
			r.experimentRuleWrite("on", "Turn an experiment on for every user", codersdk.ExperimentRuleModeOn),
			r.experimentRuleWrite("off", "Turn an experiment off for every user, even if it is enabled at startup", codersdk.ExperimentRuleModeOff),
			r.experimentRuleWrite("set", "Turn an experiment on for users whose CEL condition is true", codersdk.ExperimentRuleModeCondition),
			r.experimentRuleWrite("reset", "Restore the startup --experiments default of an experiment", codersdk.ExperimentRuleModeInherit),
		},
	}
}

type experimentRuleRow struct {
	Experiment    codersdk.Experiment `table:"experiment,default_sort"`
	StaticDefault bool                `table:"static default"`
	Mode          string              `table:"mode"`
	Condition     string              `table:"condition"`
	Revision      int64               `table:"revision"`
	UpdatedAt     string              `table:"updated at"`
	Ignored       bool                `table:"ignored"`
}

func experimentRuleRows(entries []codersdk.ExperimentRuleEntry) []experimentRuleRow {
	rows := make([]experimentRuleRow, 0, len(entries))
	for _, entry := range entries {
		row := experimentRuleRow{
			Experiment:    entry.Experiment,
			StaticDefault: entry.StaticDefault,
			Mode:          "(none)",
			Ignored:       entry.Ignored,
		}
		if entry.Rule != nil {
			row.Mode = experimentRuleModeLabel(entry.Rule.Mode)
			row.Condition = entry.Rule.Condition
			row.Revision = entry.Rule.Revision
			if !entry.Rule.UpdatedAt.IsZero() {
				row.UpdatedAt = entry.Rule.UpdatedAt.Format(time.RFC3339)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func experimentRuleModeLabel(mode codersdk.ExperimentRuleMode) string {
	if mode == "" {
		return "(malformed)"
	}
	return string(mode)
}

func (r *RootCmd) experimentRulesList() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]experimentRuleRow{}, []string{"experiment", "static default", "mode", "condition", "revision", "updated at", "ignored"}),
			func(data any) (any, error) {
				entries, ok := data.([]codersdk.ExperimentRuleEntry)
				if !ok {
					return nil, xerrors.Errorf("expected []codersdk.ExperimentRuleEntry, got %T", data)
				}
				return experimentRuleRows(entries), nil
			},
		),
		cliui.JSONFormat(),
	)
	cmd := &serpent.Command{
		Use:        "list",
		Short:      "List the runtime rules of user-scoped experiments and ignored stored rules",
		Middleware: serpent.RequireNArgs(0),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			entries, err := codersdk.NewExperimentalClient(client).ExperimentRules(inv.Context())
			if err != nil {
				return xerrors.Errorf("list experiment rules: %w", err)
			}
			out, err := formatter.Format(inv.Context(), entries)
			if err != nil {
				return xerrors.Errorf("format output: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) experimentRuleWrite(use, short string, mode codersdk.ExperimentRuleMode) *serpent.Command {
	var expectedRevision int64
	args := 1
	usage := use + " <experiment>"
	if mode == codersdk.ExperimentRuleModeCondition {
		args = 2
		usage = use + " <experiment> <condition>"
	}
	long := experimentRuleRevisionHelp
	if mode == codersdk.ExperimentRuleModeInherit {
		long = "Reset is not a kill switch: the startup default may enable the experiment. " +
			"Use off to disable it for everyone.\n\n" + long
	}
	return &serpent.Command{
		Use:        usage,
		Short:      short,
		Long:       long,
		Middleware: serpent.RequireNArgs(args),
		Options: serpent.OptionSet{
			{
				Flag:        "expected-revision",
				Description: "Write only if the stored rule is at this revision. Use 0 for an experiment that never had a rule.",
				Value:       serpent.Int64Of(&expectedRevision),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			exp := codersdk.NewExperimentalClient(client)
			ex := codersdk.Experiment(inv.Args[0])
			req := codersdk.PutExperimentRuleRequest{
				Mode:             mode,
				ExpectedRevision: expectedRevision,
			}
			if mode == codersdk.ExperimentRuleModeCondition {
				req.Condition = inv.Args[1]
			}

			revisionSet := inv.ParsedFlags().Changed("expected-revision")
			var current *codersdk.ExperimentRuleEntry
			if !revisionSet || mode == codersdk.ExperimentRuleModeInherit {
				entries, err := exp.ExperimentRules(ctx)
				if err != nil {
					return xerrors.Errorf("read experiment rules: %w", err)
				}
				current = findExperimentRuleEntry(entries, ex)
			}
			if !revisionSet && current != nil && current.Rule != nil {
				req.ExpectedRevision = current.Rule.Revision
			}

			rule, err := exp.PutExperimentRule(ctx, ex, req)
			if err != nil {
				var sdkErr *codersdk.Error
				if errors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusConflict {
					// Show what changed and stop. Retrying could silently
					// overwrite another administrator's rule.
					if entries, listErr := exp.ExperimentRules(ctx); listErr == nil {
						printCurrentExperimentRule(inv, ex, findExperimentRuleEntry(entries, ex))
					}
					return xerrors.Errorf("rule for experiment %q was not changed: %w", ex, err)
				}
				return xerrors.Errorf("write rule for experiment %q: %w", ex, err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Experiment %q rule is now %s (revision %d).\n", ex, experimentRuleModeLabel(rule.Mode), rule.Revision)
			if mode == codersdk.ExperimentRuleModeInherit && current != nil && current.StaticDefault {
				cliui.Warnf(inv.Stderr, "Experiment %q is enabled by the startup --experiments default, so it is now on for every user. Reset is not a kill switch; use off to disable it.", ex)
			}
			return nil
		},
	}
}

func findExperimentRuleEntry(entries []codersdk.ExperimentRuleEntry, ex codersdk.Experiment) *codersdk.ExperimentRuleEntry {
	for i := range entries {
		if entries[i].Experiment == ex {
			return &entries[i]
		}
	}
	return nil
}

func printCurrentExperimentRule(inv *serpent.Invocation, ex codersdk.Experiment, entry *codersdk.ExperimentRuleEntry) {
	if entry == nil || entry.Rule == nil {
		_, _ = fmt.Fprintf(inv.Stderr, "Current rule for experiment %q: none (revision 0).\n", ex)
		return
	}
	rule := entry.Rule
	_, _ = fmt.Fprintf(inv.Stderr, "Current rule for experiment %q: %s (revision %d), updated at %s by %s.\n",
		ex, experimentRuleModeLabel(rule.Mode), rule.Revision, rule.UpdatedAt.Format(time.RFC3339), rule.UpdatedBy)
	if rule.Condition != "" {
		_, _ = fmt.Fprintf(inv.Stderr, "Condition: %s\n", rule.Condition)
	}
}
