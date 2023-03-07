package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

func (pf *templateUploadFlags) register(f *pflag.FlagSet) {
	currentDirectory, _ := os.Getwd()
	f.StringVarP(&pf.directory, "directory", "d", currentDirectory, "Specify the directory to create from, use '-' to read tar from stdin")
}

func (pf *templateUploadFlags) stdin() bool {
	return pf.directory == "-"
}

func (pf *templateUploadFlags) upload(cmd *clibase.Command, client *codersdk.Client) (*codersdk.UploadResponse, error) {
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
	spin.Suffix = cliui.Styles.Keyword.Render(" Uploading directory...")
	spin.Start()
	defer spin.Stop()

	resp, err := client.Upload(inv.Context(), codersdk.ContentTypeTar, bufio.NewReader(content))
	if err != nil {
		return nil, xerrors.Errorf("upload: %w", err)
	}
	return &resp, nil
}

func (pf *templateUploadFlags) templateName(inv.Args []string) (string, error) {
	if pf.stdin() {
		// Can't infer name from directory if none provided.
		if len(inv.Args) == 0 {
			return "", xerrors.New("template name argument must be provided")
		}
		return inv.Args[0], nil
	}

	name := filepath.Base(pf.directory)
	if len(inv.Args) > 0 {
		name = inv.Args[0]
	}
	return name, nil
}

func templatePush() *clibase.Command {
	var (
		versionName     string
		provisioner     string
		parameterFile   string
		variablesFile   string
		variables       []string
		alwaysPrompt    bool
		provisionerTags []string
		uploadFlags     templateUploadFlags
	)

	cmd := &clibase.Command{
		Use:   "push [template]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Push a new template version from the current directory or as specified by flag",
		Handler: func(inv *clibase.Invokation) error {
			client, err := useClient(cmd)
			if err != nil {
				return err
			}
			organization, err := CurrentOrganization(cmd, client)
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

			resp, err := uploadFlags.upload(cmd, client)
			if err != nil {
				return err
			}

			tags, err := ParseProvisionerTags(provisionerTags)
			if err != nil {
				return err
			}

			job, _, err := createValidTemplateVersion(cmd, createValidTemplateVersionArgs{
				Name:            versionName,
				Client:          client,
				Organization:    organization,
				Provisioner:     database.ProvisionerType(provisioner),
				FileID:          resp.ID,
				ParameterFile:   parameterFile,
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

			err = client.UpdateActiveTemplateVersion(inv.Context(), template.ID, codersdk.UpdateActiveTemplateVersion{
				ID: job.ID,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Updated version at %s!\n", cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}

	cmd.Flags().StringVarP(&provisioner, "test.provisioner", "", "terraform", "Customize the provisioner backend")
	cmd.Flags().StringVarP(&parameterFile, "parameter-file", "", "", "Specify a file path with parameter values.")
	cmd.Flags().StringVarP(&variablesFile, "variables-file", "", "", "Specify a file path with values for Terraform-managed variables.")
	cmd.Flags().StringArrayVarP(&variables, "variable", "", []string{}, "Specify a set of values for Terraform-managed variables.")
	cmd.Flags().StringVarP(&versionName, "name", "", "", "Specify a name for the new template version. It will be automatically generated if not provided.")
	cmd.Flags().StringArrayVarP(&provisionerTags, "provisioner-tag", "", []string{}, "Specify a set of tags to target provisioner daemons.")
	cmd.Flags().BoolVar(&alwaysPrompt, "always-prompt", false, "Always prompt all parameters. Does not pull parameter values from active template version")
	uploadFlags.register(cmd.Flags())
	cliui.AllowSkipPrompt(inv)
	// This is for testing!
	err := inv.ParsedFlags().MarkHidden("test.provisioner")
	if err != nil {
		panic(err)
	}

	return cmd
}
