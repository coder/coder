//go:build !slim

package cli

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/scaletest/dynamicparameters"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/serpent"
)

const (
	dynamicParametersTestName = "dynamic-parameters"
)

func (r *RootCmd) scaletestDynamicParameters() *serpent.Command {
	var templateName string
	var numEvals int64
	orgContext := NewOrganizationContext()
	output := &scaletestOutputFlags{}

	cmd := &serpent.Command{
		Use:   "dynamic-parameters",
		Short: "Generates load on the Coder server evaluating dynamic parameters",
		Long:  `It is recommended that all rate limits are disabled on the server before running this scaletest. This test generates many login events which will be rate limited against the (most likely single) IP.`,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			if templateName == "" {
				return xerrors.Errorf("template cannot be empty")
			}

			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}

			logger := slog.Make(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
			partitions, err := dynamicparameters.SetupPartitions(ctx, client, org.ID, templateName, numEvals, logger)
			if err != nil {
				return xerrors.Errorf("setup dynamic parameters partitions: %w", err)
			}

			th := harness.NewTestHarness(harness.ConcurrentExecutionStrategy{}, harness.ConcurrentExecutionStrategy{})
			reg := prometheus.NewRegistry()
			metrics := dynamicparameters.NewMetrics(reg, "concurrent_evaluations")

			for i, part := range partitions {
				for j := range part.ConcurrentEvaluations {
					cfg := dynamicparameters.Config{
						TemplateVersion:   part.TemplateVersion.ID,
						Metrics:           metrics,
						MetricLabelValues: []string{fmt.Sprintf("%d", part.ConcurrentEvaluations)},
					}
					runner := dynamicparameters.NewRunner(client, cfg)
					th.AddRun(dynamicParametersTestName, fmt.Sprintf("%d/%d", j, i), runner)
				}
			}

			err = th.Run(ctx)
			if err != nil {
				return xerrors.Errorf("run test harness: %w", err)
			}

			res := th.Results()
			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "template",
			Description: "Name of the template to use. If it does not exist, it will be created.",
			Default:     "scaletest-dynamic-parameters",
			Value:       serpent.StringOf(&templateName),
		},
		{
			Flag:        "concurrent-evaluations",
			Description: "Number of concurrent dynamic parameter evaluations to perform.",
			Default:     "100",
			Value:       serpent.Int64Of(&numEvals),
		},
	}
	orgContext.AttachOptions(cmd)
	output.attach(&cmd.Options)
	return cmd
}
