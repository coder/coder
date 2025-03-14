package cli
import (
	"errors"
	"fmt"
	"slices"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)
func (r *RootCmd) provisionerJobs() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "jobs",
		Short: "View and manage provisioner jobs",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Aliases: []string{"job"},
		Children: []*serpent.Command{
			r.provisionerJobsCancel(),
			r.provisionerJobsList(),
		},
	}
	return cmd
}
func (r *RootCmd) provisionerJobsList() *serpent.Command {
	type provisionerJobRow struct {
		codersdk.ProvisionerJob `table:"provisioner_job,recursive_inline,nosort"`
		OrganizationName        string `json:"organization_name" table:"organization"`
		Queue                   string `json:"-" table:"queue"`
	}
	var (
		client     = new(codersdk.Client)
		orgContext = NewOrganizationContext()
		formatter  = cliui.NewOutputFormatter(
			cliui.TableFormat([]provisionerJobRow{}, []string{"created at", "id", "type", "template display name", "status", "queue", "tags"}),
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
				return fmt.Errorf("current organization: %w", err)
			}
			jobs, err := client.OrganizationProvisionerJobs(ctx, org.ID, &codersdk.OrganizationProvisionerJobsOptions{
				Status: slice.StringEnums[codersdk.ProvisionerJobStatus](status),
				Limit:  int(limit),
			})
			if err != nil {
				return fmt.Errorf("list provisioner jobs: %w", err)
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
			// Sort manually because the cliui table truncates timestamps and
			// produces an unstable sort with timestamps that are all the same.
			slices.SortStableFunc(rows, func(a provisionerJobRow, b provisionerJobRow) int {
				return a.CreatedAt.Compare(b.CreatedAt)
			})
			out, err := formatter.Format(ctx, rows)
			if err != nil {
				return fmt.Errorf("display provisioner daemons: %w", err)
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
			Value:         serpent.EnumArrayOf(&status, slice.ToStrings(codersdk.ProvisionerJobStatusEnums())...),
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
func (r *RootCmd) provisionerJobsCancel() *serpent.Command {
	var (
		client     = new(codersdk.Client)
		orgContext = NewOrganizationContext()
	)
	cmd := &serpent.Command{
		Use:   "cancel <job_id>",
		Short: "Cancel a provisioner job",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return fmt.Errorf("current organization: %w", err)
			}
			jobID, err := uuid.Parse(inv.Args[0])
			if err != nil {
				return fmt.Errorf("invalid job ID: %w", err)
			}
			job, err := client.OrganizationProvisionerJob(ctx, org.ID, jobID)
			if err != nil {
				return fmt.Errorf("get provisioner job: %w", err)
			}
			switch job.Type {
			case codersdk.ProvisionerJobTypeTemplateVersionDryRun:
				_, _ = fmt.Fprintf(inv.Stdout, "Canceling template version dry run job %s...\n", job.ID)
				err = client.CancelTemplateVersionDryRun(ctx, ptr.NilToEmpty(job.Input.TemplateVersionID), job.ID)
			case codersdk.ProvisionerJobTypeTemplateVersionImport:
				_, _ = fmt.Fprintf(inv.Stdout, "Canceling template version import job %s...\n", job.ID)
				err = client.CancelTemplateVersion(ctx, ptr.NilToEmpty(job.Input.TemplateVersionID))
			case codersdk.ProvisionerJobTypeWorkspaceBuild:
				_, _ = fmt.Fprintf(inv.Stdout, "Canceling workspace build job %s...\n", job.ID)
				err = client.CancelWorkspaceBuild(ctx, ptr.NilToEmpty(job.Input.WorkspaceBuildID))
			}
			if err != nil {
				return fmt.Errorf("cancel provisioner job: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stdout, "Job canceled")
			return nil
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}
