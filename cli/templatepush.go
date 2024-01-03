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

			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return err
			}

			name, err := uploadFlags.templateName(inv.Args)
			if err != nil {
				return err
			}

			job, template, err := createTemplateVersion(createTemplateVersionArgs{
				inv:                  inv,
				client:               client,
				name:                 name,
				org:                  organization,
				uploadFlags:          uploadFlags,
				provisionerTags:      provisionerTags,
				provisioner:          provisioner,
				variablesFile:        variablesFile,
				commandLineVariables: commandLineVariables,
				versionName:          versionName,
				alwaysPrompt:         alwaysPrompt,
			})
			if err != nil {
				return err
			}

			if template == nil {
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
			Flag:        "always-prompt",
			Description: "Always prompt all parameters. Does not pull parameter values from active template version.",
			Value:       clibase.BoolOf(&alwaysPrompt),
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

type createTemplateVersionArgs struct {
	inv                  *clibase.Invocation
	client               *codersdk.Client
	name                 string
	org                  codersdk.Organization
	uploadFlags          templateUploadFlags
	provisionerTags      []string
	provisioner          string
	variablesFile        string
	commandLineVariables []string
	versionName          string
	alwaysPrompt         bool
}

func createTemplateVersion(args createTemplateVersionArgs) (*codersdk.TemplateVersion, *codersdk.Template, error) {
	if utf8.RuneCountInString(args.name) >= 32 {
		return nil, nil, xerrors.Errorf("Template name must be less than 32 characters")
	}

	var createTemplate bool
	template, err := args.client.TemplateByName(args.inv.Context(), args.org.ID, args.name)
	if err != nil {
		var apiError *codersdk.Error
		if errors.As(err, &apiError) && apiError.StatusCode() != http.StatusNotFound {
			return nil, nil, err
		}
		createTemplate = true
	}

	err = args.uploadFlags.checkForLockfile(args.inv)
	if err != nil {
		return nil, nil, xerrors.Errorf("check for lockfile: %w", err)
	}

	message := args.uploadFlags.templateMessage(args.inv)

	resp, err := args.uploadFlags.upload(args.inv, args.client)
	if err != nil {
		return nil, nil, err
	}

	tags, err := ParseProvisionerTags(args.provisionerTags)
	if err != nil {
		return nil, nil, err
	}

	userVariableValues, err := ParseUserVariableValues(
		args.variablesFile,
		args.commandLineVariables)
	if err != nil {
		return nil, nil, err
	}

	versionArgs := createValidTemplateVersionArgs{
		Message:            message,
		Client:             args.client,
		Organization:       args.org,
		Provisioner:        codersdk.ProvisionerType(args.provisioner),
		FileID:             resp.ID,
		ProvisionerTags:    tags,
		UserVariableValues: userVariableValues,
	}

	if !createTemplate {
		versionArgs.Name = args.versionName
		versionArgs.Template = &template
		versionArgs.ReuseParameters = !args.alwaysPrompt
	}

	job, err := createValidTemplateVersion(args.inv, versionArgs)
	if err != nil {
		return nil, nil, err
	}

	if job.Job.Status != codersdk.ProvisionerJobSucceeded {
		return nil, nil, xerrors.Errorf("job failed: %s", job.Job.Status)
	}

	return job, &template, nil
}
