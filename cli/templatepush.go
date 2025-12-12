package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) templatePush() *serpent.Command {
	var (
		versionName          string
		provisioner          string
		workdir              string
		variablesFile        string
		commandLineVariables []string
		alwaysPrompt         bool
		provisionerTags      []string
		uploadFlags          templateUploadFlags
		activate             bool
		orgContext           = NewOrganizationContext()
	)
	cmd := &serpent.Command{
		Use:   "push [template]",
		Short: "Create or update a template from the current directory or as specified by flag",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			uploadFlags.setWorkdir(workdir)

			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}

			name, err := uploadFlags.templateName(inv)
			if err != nil {
				return err
			}

			err = codersdk.NameValid(name)
			if err != nil {
				return xerrors.Errorf("template name %q is invalid: %w", name, err)
			}

			if versionName != "" {
				err = codersdk.TemplateVersionNameValid(versionName)
				if err != nil {
					return xerrors.Errorf("template version name %q is invalid: %w", versionName, err)
				}
			}

			var createTemplate bool
			template, err := client.TemplateByName(inv.Context(), organization.ID, name)
			if err != nil {
				var apiError *codersdk.Error
				if errors.As(err, &apiError) && apiError.StatusCode() != http.StatusNotFound {
					return err
				}
				// Template doesn't exist, create it.
				createTemplate = true
			}

			var tags map[string]string
			// Passing --provisioner-tag="-" allows the user to clear all provisioner tags.
			if len(provisionerTags) == 1 && strings.TrimSpace(provisionerTags[0]) == "-" {
				cliui.Warn(inv.Stderr, "Not reusing provisioner tags from the previous template version.")
				tags = map[string]string{}
			} else {
				tags, err = ParseProvisionerTags(provisionerTags)
				if err != nil {
					return err
				}

				// If user hasn't provided new provisioner tags, inherit ones from the active template version.
				if len(tags) == 0 && template.ActiveVersionID != uuid.Nil {
					templateVersion, err := client.TemplateVersion(inv.Context(), template.ActiveVersionID)
					if err != nil {
						return err
					}
					tags = templateVersion.Job.Tags
					cliui.Info(inv.Stderr, "Re-using provisioner tags from the active template version.")
					cliui.Info(inv.Stderr, "Tip: You can override these tags by passing "+cliui.Code(`--provisioner-tag="key=value"`)+".")
					cliui.Info(inv.Stderr, "     You can also clear all provisioner tags by passing "+cliui.Code(`--provisioner-tag="-"`)+".")
				}
			}

			{ // For clarity, display provisioner tags to the user.
				var tmp []string
				for k, v := range tags {
					if k == provisionersdk.TagScope || k == provisionersdk.TagOwner {
						continue
					}
					tmp = append(tmp, fmt.Sprintf("%s=%q", k, v))
				}
				slices.Sort(tmp)
				tagStr := strings.Join(tmp, " ")
				if len(tmp) == 0 {
					tagStr = "<none>"
				}
				cliui.Info(inv.Stderr, "Provisioner tags: "+cliui.Code(tagStr))
			}

			err = uploadFlags.checkForLockfile(inv)
			if err != nil {
				return xerrors.Errorf("check for lockfile: %w", err)
			}

			message := uploadFlags.templateMessage(inv)

			var varsFiles []string
			if !uploadFlags.stdin(inv) {
				varsFiles, err = codersdk.DiscoverVarsFiles(uploadFlags.directory)
				if err != nil {
					return err
				}

				if len(varsFiles) > 0 {
					_, _ = fmt.Fprintln(inv.Stdout, "Auto-discovered Terraform tfvars files. Make sure to review and clean up any unused files.")
				}
			}

			resp, err := uploadFlags.upload(inv, client)
			if err != nil {
				return err
			}

			userVariableValues, err := codersdk.ParseUserVariableValues(
				varsFiles,
				variablesFile,
				commandLineVariables)
			if err != nil {
				return err
			}

			args := createValidTemplateVersionArgs{
				Message:            message,
				Client:             client,
				Organization:       organization,
				Provisioner:        codersdk.ProvisionerType(provisioner),
				FileID:             resp.ID,
				ProvisionerTags:    tags,
				UserVariableValues: userVariableValues,
			}

			// This ensures the version name is set in the request arguments regardless of whether you're creating a new template or updating an existing one.
			args.Name = versionName
			if !createTemplate {
				args.Template = &template
				args.ReuseParameters = !alwaysPrompt
			}

			job, err := createValidTemplateVersion(inv, args)
			if err != nil {
				return err
			}

			if job.Job.Status != codersdk.ProvisionerJobSucceeded {
				return xerrors.Errorf("job failed: %s", job.Job.Status)
			}

			if createTemplate {
				_, err = client.CreateTemplate(inv.Context(), organization.ID, codersdk.CreateTemplateRequest{
					Name:      name,
					VersionID: job.ID,
				})
				if err != nil {
					return err
				}

				_, _ = fmt.Fprintln(
					inv.Stdout, "\n"+cliui.Wrap(
						"The "+cliui.Keyword(name)+" template has been created at "+cliui.Timestamp(time.Now())+"! "+
							"Developers can provision a workspace with this template using:")+"\n")
			} else if activate {
				err = client.UpdateActiveTemplateVersion(inv.Context(), template.ID, codersdk.UpdateActiveTemplateVersion{
					ID: job.ID,
				})
				if err != nil {
					return err
				}
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Updated version %s at %s!\n",
				pretty.Sprint(cliui.DefaultStyles.Keyword, job.Name),
				pretty.Sprint(cliui.DefaultStyles.DateTimeStamp, time.Now().Format(time.Stamp)))
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "test.provisioner",
			Description: "Customize the provisioner backend.",
			Default:     "terraform",
			Value:       serpent.StringOf(&provisioner),
			// This is for testing!
			Hidden: true,
		},
		{
			Flag:        "test.workdir",
			Description: "Customize the working directory.",
			Default:     "",
			Value:       serpent.StringOf(&workdir),
			// This is for testing!
			Hidden: true,
		},
		{
			Flag:        "variables-file",
			Description: "Specify a file path with values for Terraform-managed variables.",
			Value:       serpent.StringOf(&variablesFile),
		},
		{
			Flag:        "variable",
			Description: "Specify a set of values for Terraform-managed variables.",
			Value:       serpent.StringArrayOf(&commandLineVariables),
		},
		{
			Flag:        "var",
			Description: "Alias of --variable.",
			Value:       serpent.StringArrayOf(&commandLineVariables),
		},
		{
			Flag:        "provisioner-tag",
			Description: "Specify a set of tags to target provisioner daemons. If you do not specify any tags, the tags from the active template version will be reused, if available. To remove existing tags, use --provisioner-tag=\"-\".",
			Value:       serpent.StringArrayOf(&provisionerTags),
		},
		{
			Flag:        "name",
			Description: "Specify a name for the new template version. It will be automatically generated if not provided.",
			Value:       serpent.StringOf(&versionName),
		},
		{
			Flag:        "always-prompt",
			Description: "Always prompt all parameters. Does not pull parameter values from active template version.",
			Value:       serpent.BoolOf(&alwaysPrompt),
		},
		{
			Flag:        "activate",
			Description: "Whether the new template will be marked active.",
			Default:     "true",
			Value:       serpent.BoolOf(&activate),
		},
		cliui.SkipPromptOption(),
	}
	cmd.Options = append(cmd.Options, uploadFlags.options()...)
	orgContext.AttachOptions(cmd)
	return cmd
}

type templateUploadFlags struct {
	directory      string
	ignoreLockfile bool
	message        string
}

func (pf *templateUploadFlags) options() []serpent.Option {
	return []serpent.Option{{
		Flag:          "directory",
		FlagShorthand: "d",
		Description:   "Specify the directory to create from, use '-' to read tar from stdin.",
		Default:       ".",
		Value:         serpent.StringOf(&pf.directory),
	}, {
		Flag:        "ignore-lockfile",
		Description: "Ignore warnings about not having a .terraform.lock.hcl file present in the template.",
		Default:     "false",
		Value:       serpent.BoolOf(&pf.ignoreLockfile),
	}, {
		Flag:          "message",
		FlagShorthand: "m",
		Description:   "Specify a message describing the changes in this version of the template. Messages longer than 72 characters will be displayed as truncated.",
		Value:         serpent.StringOf(&pf.message),
	}}
}

func (pf *templateUploadFlags) setWorkdir(wd string) {
	if wd == "" {
		return
	}
	if pf.directory == "" || pf.directory == "." {
		pf.directory = wd
	} else if !filepath.IsAbs(pf.directory) {
		pf.directory = filepath.Join(wd, pf.directory)
	}
}

func (pf *templateUploadFlags) stdin(inv *serpent.Invocation) (out bool) {
	defer func() {
		if out {
			inv.Logger.Info(inv.Context(), "uploading tar read from stdin")
		}
	}()
	// We read a tar from stdin if the directory is "-" or if we're not in a
	// TTY and the directory flag is unset.
	return pf.directory == "-" || (!isTTYIn(inv) && !inv.ParsedFlags().Lookup("directory").Changed)
}

func (pf *templateUploadFlags) upload(inv *serpent.Invocation, client *codersdk.Client) (*codersdk.UploadResponse, error) {
	var content io.Reader
	if pf.stdin(inv) {
		content = inv.Stdin
	} else {
		prettyDir := prettyDirectoryPath(pf.directory)
		_, err := cliui.Prompt(inv, cliui.PromptOptions{
			Text:      fmt.Sprintf("Upload %q?", prettyDir),
			IsConfirm: true,
			Default:   cliui.ConfirmYes,
		})
		if err != nil {
			return nil, err
		}

		pipeReader, pipeWriter := io.Pipe()
		go func() {
			err := provisionersdk.Tar(pipeWriter, inv.Logger, pf.directory, provisionersdk.TemplateArchiveLimit)
			_ = pipeWriter.CloseWithError(err)
		}()
		defer pipeReader.Close()
		content = pipeReader
	}

	spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	spin.Writer = inv.Stdout
	spin.Suffix = pretty.Sprint(cliui.DefaultStyles.Keyword, " Uploading directory...")
	spin.Start()
	defer spin.Stop()

	resp, err := client.Upload(inv.Context(), codersdk.ContentTypeTar, bufio.NewReader(content))
	if err != nil {
		return nil, xerrors.Errorf("upload: %w", err)
	}
	return &resp, nil
}

func (pf *templateUploadFlags) checkForLockfile(inv *serpent.Invocation) error {
	if pf.stdin(inv) || pf.ignoreLockfile {
		// Just assume there's a lockfile if reading from stdin.
		return nil
	}

	hasLockfile, err := provisionersdk.DirHasLockfile(pf.directory)
	if err != nil {
		return xerrors.Errorf("dir has lockfile: %w", err)
	}

	if !hasLockfile {
		cliui.Warn(inv.Stdout, "No .terraform.lock.hcl file found",
			"When provisioning, Coder will be unable to cache providers without a lockfile and must download them from the internet each time.",
			"Create one by running "+pretty.Sprint(cliui.DefaultStyles.Code, "terraform init")+" in your template directory.",
		)
	}
	return nil
}

func (pf *templateUploadFlags) templateMessage(inv *serpent.Invocation) string {
	title := strings.SplitN(pf.message, "\n", 2)[0]
	if len(title) > 72 {
		cliui.Warn(inv.Stdout, "Template message is longer than 72 characters, it will be displayed as truncated.")
	}
	if title != pf.message {
		cliui.Warn(inv.Stdout, "Template message contains newlines, only the first line will be displayed.")
	}
	if pf.message != "" {
		return pf.message
	}
	return "Uploaded from the CLI"
}

func (pf *templateUploadFlags) templateName(inv *serpent.Invocation) (string, error) {
	args := inv.Args
	if pf.stdin(inv) {
		// Can't infer name from directory if none provided.
		if len(args) == 0 {
			return "", xerrors.New("template name argument must be provided")
		}
		return args[0], nil
	}

	if len(args) > 0 {
		return args[0], nil
	}
	// Have to take absPath to resolve "." and "..".
	absPath, err := filepath.Abs(pf.directory)
	if err != nil {
		return "", err
	}
	// If no name is provided, use the directory name.
	return filepath.Base(absPath), nil
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

func createValidTemplateVersion(inv *serpent.Invocation, args createValidTemplateVersionArgs) (*codersdk.TemplateVersion, error) {
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
	cliutil.WarnMatchedProvisioners(inv.Stderr, version.MatchedProvisioners, version.Job)
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
		if errors.As(err, &jobErr) {
			if codersdk.JobIsMissingRequiredTemplateVariableErrorCode(jobErr.Code) {
				return handleMissingTemplateVariables(inv, args, version.ID)
			}
			if !codersdk.JobIsMissingParameterErrorCode(jobErr.Code) {
				return nil, err
			}
		}
		return nil, err
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

// prettyDirectoryPath returns a prettified path when inside the users
// home directory. Falls back to dir if the users home directory cannot
// discerned. This function calls filepath.Clean on the result.
func prettyDirectoryPath(dir string) string {
	dir = filepath.Clean(dir)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return dir
	}
	prettyDir := dir
	if strings.HasPrefix(prettyDir, homeDir) {
		prettyDir = strings.TrimPrefix(prettyDir, homeDir)
		prettyDir = "~" + prettyDir
	}
	return prettyDir
}

func handleMissingTemplateVariables(inv *serpent.Invocation, args createValidTemplateVersionArgs, failedVersionID uuid.UUID) (*codersdk.TemplateVersion, error) {
	client := args.Client

	templateVariables, err := client.TemplateVersionVariables(inv.Context(), failedVersionID)
	if err != nil {
		return nil, xerrors.Errorf("fetch template variables: %w", err)
	}

	existingValues := make(map[string]string)
	for _, v := range args.UserVariableValues {
		existingValues[v.Name] = v.Value
	}

	var missingVariables []codersdk.TemplateVersionVariable
	for _, variable := range templateVariables {
		if !variable.Required {
			continue
		}

		if existingValue, exists := existingValues[variable.Name]; exists && existingValue != "" {
			continue
		}

		// Only prompt for variables that don't have a default value or have a redacted default
		// Sensitive variables have a default value of "*redacted*"
		// See: https://github.com/coder/coder/blob/a78790c632974e04babfef6de0e2ddf044787a7a/coderd/provisionerdserver/provisionerdserver.go#L3206
		if variable.DefaultValue == "" || (variable.Sensitive && variable.DefaultValue == "*redacted*") {
			missingVariables = append(missingVariables, variable)
		}
	}

	if len(missingVariables) == 0 {
		return nil, xerrors.New("no missing required variables found")
	}

	_, _ = fmt.Fprintf(inv.Stderr, "Found %d missing required variables:\n", len(missingVariables))
	for _, v := range missingVariables {
		_, _ = fmt.Fprintf(inv.Stderr, "  - %s (%s): %s\n", v.Name, v.Type, v.Description)
	}

	_, _ = fmt.Fprintln(inv.Stderr, "\nThe template requires values for the following variables:")

	var promptedValues []codersdk.VariableValue
	for _, variable := range missingVariables {
		value, err := promptForTemplateVariable(inv, variable)
		if err != nil {
			return nil, xerrors.Errorf("prompt for variable %q: %w", variable.Name, err)
		}
		promptedValues = append(promptedValues, codersdk.VariableValue{
			Name:  variable.Name,
			Value: value,
		})
	}

	combinedValues := codersdk.CombineVariableValues(args.UserVariableValues, promptedValues)

	_, _ = fmt.Fprintln(inv.Stderr, "\nRetrying template build with provided variables...")

	retryArgs := args
	retryArgs.UserVariableValues = combinedValues

	return createValidTemplateVersion(inv, retryArgs)
}

func promptForTemplateVariable(inv *serpent.Invocation, variable codersdk.TemplateVersionVariable) (string, error) {
	displayVariableInfo(inv, variable)

	switch variable.Type {
	case "bool":
		return promptForBoolVariable(inv, variable)
	case "number":
		return promptForNumberVariable(inv, variable)
	default:
		return promptForStringVariable(inv, variable)
	}
}

func displayVariableInfo(inv *serpent.Invocation, variable codersdk.TemplateVersionVariable) {
	_, _ = fmt.Fprintf(inv.Stderr, "var.%s", cliui.Bold(variable.Name))
	if variable.Required {
		_, _ = fmt.Fprint(inv.Stderr, pretty.Sprint(cliui.DefaultStyles.Error, " (required)"))
	}
	if variable.Sensitive {
		_, _ = fmt.Fprint(inv.Stderr, pretty.Sprint(cliui.DefaultStyles.Warn, ", sensitive"))
	}
	_, _ = fmt.Fprintln(inv.Stderr, "")

	if variable.Description != "" {
		_, _ = fmt.Fprintf(inv.Stderr, "  Description: %s\n", variable.Description)
	}
	_, _ = fmt.Fprintf(inv.Stderr, "  Type: %s\n", variable.Type)
	_, _ = fmt.Fprintf(inv.Stderr, "  Current value: %s\n", pretty.Sprint(cliui.DefaultStyles.Placeholder, "<empty>"))
}

func promptForBoolVariable(inv *serpent.Invocation, variable codersdk.TemplateVersionVariable) (string, error) {
	defaultValue := variable.DefaultValue
	if defaultValue == "" {
		defaultValue = "false"
	}

	return cliui.Select(inv, cliui.SelectOptions{
		Options: []string{"true", "false"},
		Default: defaultValue,
		Message: "Select value:",
	})
}

func promptForNumberVariable(inv *serpent.Invocation, variable codersdk.TemplateVersionVariable) (string, error) {
	prompt := "Enter value:"
	if !variable.Required && variable.DefaultValue != "" {
		prompt = fmt.Sprintf("Enter value (default: %q):", variable.DefaultValue)
	}

	return cliui.Prompt(inv, cliui.PromptOptions{
		Text:     prompt,
		Default:  variable.DefaultValue,
		Validate: createVariableValidator(variable),
	})
}

func promptForStringVariable(inv *serpent.Invocation, variable codersdk.TemplateVersionVariable) (string, error) {
	prompt := "Enter value:"
	if !variable.Sensitive {
		if !variable.Required && variable.DefaultValue != "" {
			prompt = fmt.Sprintf("Enter value (default: %q):", variable.DefaultValue)
		}
	}

	return cliui.Prompt(inv, cliui.PromptOptions{
		Text:     prompt,
		Default:  variable.DefaultValue,
		Secret:   variable.Sensitive,
		Validate: createVariableValidator(variable),
	})
}

func createVariableValidator(variable codersdk.TemplateVersionVariable) func(string) error {
	return func(s string) error {
		if variable.Required && s == "" && variable.DefaultValue == "" {
			return xerrors.New("value is required")
		}
		if variable.Type == "number" && s != "" {
			if _, err := strconv.ParseFloat(s, 64); err != nil {
				return xerrors.Errorf("must be a valid number, got: %q", s)
			}
		}
		return nil
	}
}
