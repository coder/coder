package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/briandowns/spinner"
	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

// templateUploadFlags is shared by `templates create` and `templates push`.
type templateUploadFlags struct {
	directory      string
	ignoreLockfile bool
	message        string
}

func (pf *templateUploadFlags) options() []clibase.Option {
	return []clibase.Option{{
		Flag:          "directory",
		FlagShorthand: "d",
		Description:   "Specify the directory to create from, use '-' to read tar from stdin.",
		Default:       ".",
		Value:         clibase.StringOf(&pf.directory),
	}, {
		Flag:        "ignore-lockfile",
		Description: "Ignore warnings about not having a .terraform.lock.hcl file present in the template.",
		Default:     "false",
		Value:       clibase.BoolOf(&pf.ignoreLockfile),
	}, {
		Flag:          "message",
		FlagShorthand: "m",
		Description:   "Specify a message describing the changes in this version of the template. Messages longer than 72 characters will be displayed as truncated.",
		Value:         clibase.StringOf(&pf.message),
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

func (pf *templateUploadFlags) stdin() bool {
	return pf.directory == "-"
}

func (pf *templateUploadFlags) upload(inv *clibase.Invocation, client *codersdk.Client) (*codersdk.UploadResponse, error) {
	var content io.Reader
	if pf.stdin() {
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

func (pf *templateUploadFlags) checkForLockfile(inv *clibase.Invocation) error {
	if pf.stdin() || pf.ignoreLockfile {
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

func (pf *templateUploadFlags) templateMessage(inv *clibase.Invocation) string {
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

func (pf *templateUploadFlags) templateName(args []string) (string, error) {
	if pf.stdin() {
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

func (r *RootCmd) templatePush() *clibase.Cmd {
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

		displayName                    string
		description                    string
		icon                           string
		requireActiveVersion           bool
		disableEveryone                bool
		defaultTTL                     time.Duration
		failureTTL                     time.Duration
		dormancyThreshold              time.Duration
		dormancyAutoDeletion           time.Duration
		maxTTL                         time.Duration
		autostopRequirementDaysOfWeek  []string
		autostopRequirementWeeks       int64
		autostartRequirementDaysOfWeek []string
		allowUserAutostart             bool
		allowUserAutostop              bool
		allowUserCancelWorkspaceJobs   bool
		deprecationMessage             string
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "push [template]",
		Short: "Push a new template version from the current directory or as specified by flag",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(0, 1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			uploadFlags.setWorkdir(workdir)

			err := createEntitlementsCheck(inv.Context(), handleEntitlementsArgs{
				client:               client,
				requireActiveVersion: requireActiveVersion,
				defaultTTL:           defaultTTL,
				failureTTL:           failureTTL,
				dormancyThreshold:    dormancyThreshold,
				dormancyAutoDeletion: dormancyAutoDeletion,
				maxTTL:               maxTTL,
			})
			if err != nil {
				return err
			}

			unsetAutostopRequirementDaysOfWeek, err := editTemplateEntitlementsCheck(inv.Context(), editTemplateEntitlementsArgs{
				client:                         client,
				inv:                            inv,
				defaultTTL:                     defaultTTL,
				maxTTL:                         maxTTL,
				autostopRequirementDaysOfWeek:  autostopRequirementDaysOfWeek,
				autostopRequirementWeeks:       autostopRequirementWeeks,
				autostartRequirementDaysOfWeek: autostartRequirementDaysOfWeek,
				failureTTL:                     failureTTL,
				dormancyThreshold:              dormancyThreshold,
				dormancyAutoDeletion:           dormancyAutoDeletion,
				allowUserCancelWorkspaceJobs:   allowUserCancelWorkspaceJobs,
				allowUserAutostart:             allowUserAutostart,
				allowUserAutostop:              allowUserAutostop,
				requireActiveVersion:           requireActiveVersion,
			})
			if err != nil {
				return err
			}

			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return err
			}

			name, err := uploadFlags.templateName(inv.Args)
			if err != nil {
				return err
			}

			if utf8.RuneCountInString(name) > 31 {
				return xerrors.Errorf("Template name must be less than 32 characters")
			}

			var createTemplate bool
			template, err := client.TemplateByName(inv.Context(), organization.ID, name)
			if err != nil {
				var apiError *codersdk.Error
				if errors.As(err, &apiError) && apiError.StatusCode() != http.StatusNotFound {
					return err
				}
				createTemplate = true
			}

			err = uploadFlags.checkForLockfile(inv)
			if err != nil {
				return xerrors.Errorf("check for lockfile: %w", err)
			}

			message := uploadFlags.templateMessage(inv)

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

			args := createValidTemplateVersionArgs{
				Message:            message,
				Client:             client,
				Organization:       organization,
				Provisioner:        codersdk.ProvisionerType(provisioner),
				FileID:             resp.ID,
				ProvisionerTags:    tags,
				UserVariableValues: userVariableValues,
			}

			if !createTemplate {
				args.Name = versionName
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

			_, _ = fmt.Fprintf(inv.Stdout, "Updated version at %s!\n", pretty.Sprint(cliui.DefaultStyles.DateTimeStamp, time.Now().Format(time.Stamp)))

			// refresh template data for edit api call
			template, err = client.TemplateByName(inv.Context(), organization.ID, name)
			if err != nil {
				return err
			}
			req := updateTemplateMetaRequest(updateTemplateMetaArgs{
				client:                             client,
				inv:                                inv,
				template:                           template,
				unsetAutostopRequirementDaysOfWeek: unsetAutostopRequirementDaysOfWeek,

				displayName:                    displayName,
				description:                    description,
				icon:                           icon,
				requireActiveVersion:           requireActiveVersion,
				disableEveryone:                disableEveryone,
				defaultTTL:                     defaultTTL,
				failureTTL:                     failureTTL,
				dormancyThreshold:              dormancyThreshold,
				dormancyAutoDeletion:           dormancyAutoDeletion,
				maxTTL:                         maxTTL,
				autostopRequirementDaysOfWeek:  autostopRequirementDaysOfWeek,
				autostopRequirementWeeks:       autostopRequirementWeeks,
				autostartRequirementDaysOfWeek: autostartRequirementDaysOfWeek,
				allowUserAutostart:             allowUserAutostart,
				allowUserAutostop:              allowUserAutostop,
				allowUserCancelWorkspaceJobs:   allowUserCancelWorkspaceJobs,
				deprecationMessage:             deprecationMessage,
			})

			_, err = client.UpdateTemplateMeta(inv.Context(), template.ID, req)
			if err != nil {
				return xerrors.Errorf("update template metadata: %w", err)
			}
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Updated template metadata at %s!\n", pretty.Sprint(cliui.DefaultStyles.DateTimeStamp, time.Now().Format(time.Stamp)))

			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:        "test.provisioner",
			Description: "Customize the provisioner backend.",
			Default:     "terraform",
			Value:       clibase.StringOf(&provisioner),
			// This is for testing!
			Hidden: true,
		},
		{
			Flag:        "test.workdir",
			Description: "Customize the working directory.",
			Default:     "",
			Value:       clibase.StringOf(&workdir),
			// This is for testing!
			Hidden: true,
		},
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
			Flag:        "name",
			Description: "Specify a name for the new template version. It will be automatically generated if not provided.",
			Value:       clibase.StringOf(&versionName),
		},
		{
			Flag:        "display-name",
			Description: "Edit the template display name.",
			Value:       clibase.StringOf(&displayName),
		},
		{
			Flag:        "description",
			Description: "Edit the template description.",
			Value:       clibase.StringOf(&description),
		},
		{
			Name:        "Deprecated",
			Flag:        "deprecated",
			Description: "Sets the template as deprecated. Must be a message explaining why the template is deprecated.",
			Value:       clibase.StringOf(&deprecationMessage),
		},
		{
			Flag:        "icon",
			Description: "Edit the template icon path.",
			Value:       clibase.StringOf(&icon),
		},
		{
			Flag:        "always-prompt",
			Description: "Always prompt all parameters. Does not pull parameter values from active template version.",
			Value:       clibase.BoolOf(&alwaysPrompt),
		},
		{
			Flag:        "activate",
			Description: "Whether the new template will be marked active.",
			Default:     "true",
			Value:       clibase.BoolOf(&activate),
		},
		{
			Flag:        "require-active-version",
			Description: "Requires workspace builds to use the active template version. This setting does not apply to template admins. This is an enterprise-only feature.",
			Value:       clibase.BoolOf(&requireActiveVersion),
			Default:     "false",
		},
		{
			Flag:        "default-ttl",
			Description: "Specify a default TTL for workspaces created from this template. It is the default time before shutdown - workspaces created from this template default to this value. Maps to \"Default autostop\" in the UI.",
			Default:     "24h",
			Value:       clibase.DurationOf(&defaultTTL),
		},
		{
			Flag:        "failure-ttl",
			Description: "Specify a failure TTL for workspaces created from this template. It is the amount of time after a failed \"start\" build before coder automatically schedules a \"stop\" build to cleanup.This licensed feature's default is 0h (off). Maps to \"Failure cleanup\"in the UI.",
			Default:     "0h",
			Value:       clibase.DurationOf(&failureTTL),
		},
		{
			Flag:        "dormancy-threshold",
			Description: "Specify a duration workspaces may be inactive prior to being moved to the dormant state. This licensed feature's default is 0h (off). Maps to \"Dormancy threshold\" in the UI.",
			Default:     "0h",
			Value:       clibase.DurationOf(&dormancyThreshold),
		},
		{
			Flag:        "dormancy-auto-deletion",
			Description: "Specify a duration workspaces may be in the dormant state prior to being deleted. This licensed feature's default is 0h (off). Maps to \"Dormancy Auto-Deletion\" in the UI.",
			Default:     "0h",
			Value:       clibase.DurationOf(&dormancyAutoDeletion),
		},
		{
			Flag:        "max-ttl",
			Description: "Edit the template maximum time before shutdown - workspaces created from this template must shutdown within the given duration after starting. This is an enterprise-only feature.",
			Value:       clibase.DurationOf(&maxTTL),
		},
		{
			Flag: "private",
			Description: "Disable the default behavior of granting template access to the 'everyone' group. " +
				"The template permissions must be updated to allow non-admin users to use this template.",
			Value: clibase.BoolOf(&disableEveryone),
		},
		{
			Flag: "autostart-requirement-weekdays",
			// workspaces created from this template must be restarted on the given weekdays. To unset this value for the template (and disable the autostop requirement for the template), pass 'none'.
			Description: "Edit the template autostart requirement weekdays - workspaces created from this template can only autostart on the given weekdays. To unset this value for the template (and allow autostart on all days), pass 'all'.",
			Value: clibase.Validate(clibase.StringArrayOf(&autostartRequirementDaysOfWeek), func(value *clibase.StringArray) error {
				v := value.GetSlice()
				if len(v) == 1 && v[0] == "all" {
					return nil
				}
				_, err := codersdk.WeekdaysToBitmap(v)
				if err != nil {
					return xerrors.Errorf("invalid autostart requirement days of week %q: %w", strings.Join(v, ","), err)
				}
				return nil
			}),
		},
		{
			Flag:        "autostop-requirement-weekdays",
			Description: "Edit the template autostop requirement weekdays - workspaces created from this template must be restarted on the given weekdays. To unset this value for the template (and disable the autostop requirement for the template), pass 'none'.",
			// TODO(@dean): unhide when we delete max_ttl
			Hidden: true,
			Value: clibase.Validate(clibase.StringArrayOf(&autostopRequirementDaysOfWeek), func(value *clibase.StringArray) error {
				v := value.GetSlice()
				if len(v) == 1 && v[0] == "none" {
					return nil
				}
				_, err := codersdk.WeekdaysToBitmap(v)
				if err != nil {
					return xerrors.Errorf("invalid autostop requirement days of week %q: %w", strings.Join(v, ","), err)
				}
				return nil
			}),
		},
		{
			Flag:        "autostop-requirement-weeks",
			Description: "Edit the template autostop requirement weeks - workspaces created from this template must be restarted on an n-weekly basis.",
			// TODO(@dean): unhide when we delete max_ttl
			Hidden: true,
			Value:  clibase.Int64Of(&autostopRequirementWeeks),
		},
		{
			Flag:        "allow-user-cancel-workspace-jobs",
			Description: "Allow users to cancel in-progress workspace jobs.",
			Default:     "true",
			Value:       clibase.BoolOf(&allowUserCancelWorkspaceJobs),
		},
		{
			Flag:        "allow-user-autostart",
			Description: "Allow users to configure autostart for workspaces on this template. This can only be disabled in enterprise.",
			Default:     "true",
			Value:       clibase.BoolOf(&allowUserAutostart),
		},
		{
			Flag:        "allow-user-autostop",
			Description: "Allow users to customize the autostop TTL for workspaces on this template. This can only be disabled in enterprise.",
			Default:     "true",
			Value:       clibase.BoolOf(&allowUserAutostop),
		},
		cliui.SkipPromptOption(),
	}
	cmd.Options = append(cmd.Options, uploadFlags.options()...)
	return cmd
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
