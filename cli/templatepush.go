package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/xerrors"

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
			err := provisionersdk.Tar(pipeWriter, pf.directory, provisionersdk.TemplateArchiveLimit)
			_ = pipeWriter.CloseWithError(err)
		}()
		defer pipeReader.Close()
		content = pipeReader
	}

	spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	spin.Writer = inv.Stdout
	spin.Suffix = cliui.DefaultStyles.Keyword.Render(" Uploading directory...")
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
			"Create one by running "+cliui.DefaultStyles.Code.Render("terraform init")+" in your template directory.",
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
		versionName     string
		provisioner     string
		workdir         string
		variablesFile   string
		variables       []string
		alwaysPrompt    bool
		provisionerTags []string
		uploadFlags     templateUploadFlags
		activate        bool
		create          bool
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

			var createTemplate bool
			template, err := client.TemplateByName(inv.Context(), organization.ID, name)
			if err != nil {
				if !create {
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

			args := createValidTemplateVersionArgs{
				Message:         message,
				Client:          client,
				Organization:    organization,
				Provisioner:     codersdk.ProvisionerType(provisioner),
				FileID:          resp.ID,
				ProvisionerTags: tags,
				VariablesFile:   variablesFile,
				Variables:       variables,
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

				_, _ = fmt.Fprintln(inv.Stdout, "\n"+cliui.DefaultStyles.Wrap.Render(
					"The "+cliui.DefaultStyles.Keyword.Render(name)+" template has been created at "+cliui.DefaultStyles.DateTimeStamp.Render(time.Now().Format(time.Stamp))+"! "+
						"Developers can provision a workspace with this template using:")+"\n")
			} else if activate {
				err = client.UpdateActiveTemplateVersion(inv.Context(), template.ID, codersdk.UpdateActiveTemplateVersion{
					ID: job.ID,
				})
				if err != nil {
					return err
				}
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Updated version at %s!\n", cliui.DefaultStyles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
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
			Value:       clibase.StringArrayOf(&variables),
		},
		{
			Flag:        "var",
			Description: "Alias of --variable.",
			Value:       clibase.StringArrayOf(&variables),
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
		{
			Flag:        "activate",
			Description: "Whether the new template will be marked active.",
			Default:     "true",
			Value:       clibase.BoolOf(&activate),
		},
		{
			Flag:        "create",
			Description: "Create the template if it does not exist.",
			Default:     "false",
			Value:       clibase.BoolOf(&create),
		},
		cliui.SkipPromptOption(),
	}
	cmd.Options = append(cmd.Options, uploadFlags.options()...)
	return cmd
}
