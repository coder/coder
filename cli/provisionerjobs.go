package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) provisionerJobs() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "jobs",
		Short: "View and manage provisioner jobs.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Aliases: []string{"job"},
		Children: []*serpent.Command{
			r.provisionerJobsList(),
		},
	}
	return cmd
}

func (r *RootCmd) provisionerJobsList() *serpent.Command {
	type provisionerJobRow struct {
		codersdk.ProvisionerJob `table:"provisioner_job,recursive_inline"`
		OrganizationName        string `json:"organization_name" table:"organization"`
		Queue                   string `json:"-" table:"queue"`
	}

	var (
		client     = new(codersdk.Client)
		orgContext = NewOrganizationContext()
		formatter  = cliui.NewOutputFormatter(
			cliui.TableFormat([]provisionerJobRow{}, []string{"created at", "id", "organization", "status", "type", "queue", "tags"}),
			cliui.JSONFormat(),
		)
		status []string
		limit  int64
	)

	cmd := &serpent.Command{
		Use:     "list",
		Short:   "List provisioner jobs",
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			jobs, err := client.OrganizationProvisionerJobs(ctx, org.ID, &codersdk.OrganizationProvisionerJobsOptions{
				Status: convertSlice([]codersdk.ProvisionerJobStatus{}, status),
				Limit:  int(limit),
			})
			if err != nil {
				return xerrors.Errorf("list provisioner jobs: %w", err)
			}

			if len(jobs) == 0 {
				_, _ = fmt.Fprintln(inv.Stdout, "No provisioner jobs found")
				return nil
			}

			var rows []provisionerJobRow
			for _, job := range jobs {
				row := provisionerJobRow{
					ProvisionerJob:   job,
					OrganizationName: org.HumanName(),
				}
				if job.Status == codersdk.ProvisionerJobPending {
					row.Queue = fmt.Sprintf("%d/%d", job.QueuePosition, job.QueueSize)
				}
				rows = append(rows, row)
			}

			out, err := formatter.Format(ctx, rows)
			if err != nil {
				return xerrors.Errorf("display provisioner daemons: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, out)

			return nil
		},
	}

	cmd.Options = append(cmd.Options, []serpent.Option{
		{
			Flag:          "status",
			FlagShorthand: "s",
			Env:           "CODER_PROVISIONER_JOB_LIST_STATUS",
			Description:   "Filter by job status.",
			Value:         serpent.EnumArrayOf(&status, convertSlice([]string{}, codersdk.ProvisionerJobStatusEnums())...),
		},
		{
			Flag:          "limit",
			FlagShorthand: "l",
			Env:           "CODER_PROVISIONER_JOB_LIST_LIMIT",
			Description:   "Limit the number of jobs returned.",
			Default:       "50",
			Value:         serpent.Int64Of(&limit),
		},
	}...)

	orgContext.AttachOptions(cmd)
	formatter.AttachOptions(&cmd.Options)

	return cmd
}

func convertSlice[D, S ~string](dstType []D, src []S) []D {
	for _, item := range src {
		dstType = append(dstType, D(item))
	}
	return dstType
}
