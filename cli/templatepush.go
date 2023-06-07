package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionersdk"
)

// templateUploadFlags is shared by `templates create` and `templates push`.
type templateUploadFlags struct {
	directory string
}

func (pf *templateUploadFlags) option() clibase.Option {
	return clibase.Option{
		Flag:          "directory",
		FlagShorthand: "d",
		Description:   "Specify the directory to create from, use '-' to read tar from stdin.",
		Default:       ".",
		Value:         clibase.StringOf(&pf.directory),
	}
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

			template, err := client.TemplateByName(inv.Context(), organization.ID, name)
			if err != nil {
				return err
			}

			resp, err := uploadFlags.upload(inv, client)
			if err != nil {
				return err
			}

			tags, err := ParseProvisionerTags(provisionerTags)
			if err != nil {
				return err
			}

			job, err := createValidTemplateVersion(inv, createValidTemplateVersionArgs{
				Name:            versionName,
				Client:          client,
				Organization:    organization,
				Provisioner:     database.ProvisionerType(provisioner),
				FileID:          resp.ID,
				VariablesFile:   variablesFile,
				Variables:       variables,
				Template:        &template,
				ReuseParameters: !alwaysPrompt,
				ProvisionerTags: tags,
			})
			if err != nil {
				return err
			}

			if job.Job.Status != codersdk.ProvisionerJobSucceeded {
				return xerrors.Errorf("job failed: %s", job.Job.Status)
			}

			if activate {
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
		cliui.SkipPromptOption(),
		uploadFlags.option(),
	}
	return cmd
}
