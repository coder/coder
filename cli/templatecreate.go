package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) templateCreate() *clibase.Cmd {
	var (
		provisioner          string
		provisionerTags      []string
		variablesFile        string
		commandLineVariables []string

		uploadFlags templateUploadFlags
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "create [name]",
		Short: "Create a template from the current directory or as specified by flag",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(0, 1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			_, _ = fmt.Fprintln(inv.Stdout, "\n"+pretty.Sprint(cliui.DefaultStyles.Wrap,
				pretty.Sprint(
					cliui.DefaultStyles.Warn, "DEPRECATION WARNING: The `coder templates push` command should be used instead. This command will be removed in a future release. ")+"\n"))
			time.Sleep(1 * time.Second)

			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return err
			}

			templateName, err := uploadFlags.templateName(inv.Args)
			if err != nil {
				return err
			}

			if utf8.RuneCountInString(templateName) > 31 {
				return xerrors.Errorf("Template name must be less than 32 characters")
			}

			_, err = client.TemplateByName(inv.Context(), organization.ID, templateName)
			if err == nil {
				return xerrors.Errorf("A template already exists named %q!", templateName)
			}

			err = uploadFlags.checkForLockfile(inv)
			if err != nil {
				return xerrors.Errorf("check for lockfile: %w", err)
			}

			message := uploadFlags.templateMessage(inv)

			// Confirm upload of the directory.
			resp, err := uploadFlags.upload(inv, client)
			if err != nil {
				return err
			}

			tags, err := ParseProvisionerTags(provisionerTags)
			if err != nil {
				return err
			}

			userVariableValues, err := ParseUserVariableValues(
				variablesFile,
				commandLineVariables)
			if err != nil {
				return err
			}

			job, err := createValidTemplateVersion(inv, createValidTemplateVersionArgs{
				Message:            message,
				Client:             client,
				Organization:       organization,
				Provisioner:        codersdk.ProvisionerType(provisioner),
				FileID:             resp.ID,
				ProvisionerTags:    tags,
				UserVariableValues: userVariableValues,
			})
			if err != nil {
				return err
			}

			if !uploadFlags.stdin() {
				_, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "Confirm create?",
					IsConfirm: true,
				})
				if err != nil {
					return err
				}
			}

			createReq := codersdk.CreateTemplateRequest{
				Name:      templateName,
				VersionID: job.ID,
			}

			_, err = client.CreateTemplate(inv.Context(), organization.ID, createReq)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(inv.Stdout, "\n"+pretty.Sprint(cliui.DefaultStyles.Wrap,
				"The "+pretty.Sprint(
					cliui.DefaultStyles.Keyword, templateName)+" template has been created at "+
					pretty.Sprint(cliui.DefaultStyles.DateTimeStamp, time.Now().Format(time.Stamp))+"! "+
					"Developers can provision a workspace with this template using:")+"\n")

			_, _ = fmt.Fprintln(inv.Stdout, "  "+pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("coder create --template=%q [workspace name]", templateName)))
			_, _ = fmt.Fprintln(inv.Stdout)

			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:        "variables-file",
			Description: "Specify a file path with values for Terraform-managed variables.",
			Value:       clibase.StringOf(&variablesFile),
		},
		{
			Flag:        "variable",
			Description: "Specify a set of values for Terraform-managed variables.",
			Value:       clibase.StringArrayOf(&commandLineVariables),
		},
		{
			Flag:        "var",
			Description: "Alias of --variable.",
			Value:       clibase.StringArrayOf(&commandLineVariables),
		},
		{
			Flag:        "provisioner-tag",
			Description: "Specify a set of tags to target provisioner daemons.",
			Value:       clibase.StringArrayOf(&provisionerTags),
		},
		{
			Flag:        "test.provisioner",
			Description: "Customize the provisioner backend.",
			Default:     "terraform",
			Value:       clibase.StringOf(&provisioner),
			Hidden:      true,
		},

		cliui.SkipPromptOption(),
	}
	cmd.Options = append(cmd.Options, uploadFlags.options()...)
	return cmd
}

type createValidTemplateVersionArgs struct {
	Name         string
	Message      string
	Client       *codersdk.Client
	Organization codersdk.Organization
	Provisioner  codersdk.ProvisionerType
	FileID       uuid.UUID

	// Template is only required if updating a template's active version.
	Template *codersdk.Template
	// ReuseParameters will attempt to reuse params from the Template field
	// before prompting the user. Set to false to always prompt for param
	// values.
	ReuseParameters    bool
	ProvisionerTags    map[string]string
	UserVariableValues []codersdk.VariableValue
}

func createValidTemplateVersion(inv *clibase.Invocation, args createValidTemplateVersionArgs) (*codersdk.TemplateVersion, error) {
	client := args.Client

	req := codersdk.CreateTemplateVersionRequest{
		Name:               args.Name,
		Message:            args.Message,
		StorageMethod:      codersdk.ProvisionerStorageMethodFile,
		FileID:             args.FileID,
		Provisioner:        args.Provisioner,
		ProvisionerTags:    args.ProvisionerTags,
		UserVariableValues: args.UserVariableValues,
	}
	if args.Template != nil {
		req.TemplateID = args.Template.ID
	}
	version, err := client.CreateTemplateVersion(inv.Context(), args.Organization.ID, req)
	if err != nil {
		return nil, err
	}

	err = cliui.ProvisionerJob(inv.Context(), inv.Stdout, cliui.ProvisionerJobOptions{
		Fetch: func() (codersdk.ProvisionerJob, error) {
			version, err := client.TemplateVersion(inv.Context(), version.ID)
			return version.Job, err
		},
		Cancel: func() error {
			return client.CancelTemplateVersion(inv.Context(), version.ID)
		},
		Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
			return client.TemplateVersionLogsAfter(inv.Context(), version.ID, 0)
		},
	})
	if err != nil {
		var jobErr *cliui.ProvisionerJobError
		if errors.As(err, &jobErr) && !codersdk.JobIsMissingParameterErrorCode(jobErr.Code) {
			return nil, err
		}
		if err != nil {
			return nil, err
		}
	}
	version, err = client.TemplateVersion(inv.Context(), version.ID)
	if err != nil {
		return nil, err
	}

	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		return nil, xerrors.New(version.Job.Error)
	}

	resources, err := client.TemplateVersionResources(inv.Context(), version.ID)
	if err != nil {
		return nil, err
	}

	// Only display the resources on the start transition, to avoid listing them more than once.
	var startResources []codersdk.WorkspaceResource
	for _, r := range resources {
		if r.Transition == codersdk.WorkspaceTransitionStart {
			startResources = append(startResources, r)
		}
	}
	err = cliui.WorkspaceResources(inv.Stdout, startResources, cliui.WorkspaceResourcesOptions{
		HideAgentState: true,
		HideAccess:     true,
		Title:          "Template Preview",
	})
	if err != nil {
		return nil, xerrors.Errorf("preview template resources: %w", err)
	}

	return &version, nil
}

func ParseProvisionerTags(rawTags []string) (map[string]string, error) {
	tags := map[string]string{}
	for _, rawTag := range rawTags {
		parts := strings.SplitN(rawTag, "=", 2)
		if len(parts) < 2 {
			return nil, xerrors.Errorf("invalid tag format for %q. must be key=value", rawTag)
		}
		tags[parts[0]] = parts[1]
	}
	return tags, nil
}
