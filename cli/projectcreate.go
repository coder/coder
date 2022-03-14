package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/pion/webrtc/v3"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"golang.org/x/xerrors"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/provisionerd"
)

func projectCreate() *cobra.Command {
	var (
		directory   string
		provisioner string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project from the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			templates, err := client.Templates(cmd.Context())
			if err != nil {
				return err
			}

			items := make([]cliui.ListItem, 0)
			for _, template := range templates {
				items = append(items, cliui.ListItem{
					ID:          template.ID,
					Title:       template.Name,
					Description: template.Description,
				})
			}
			selectedItem, err := cliui.List(cmd, cliui.ListOptions{
				Title: "Select a Template",
				Items: items,
			})
			if err != nil {
				if errors.Is(err, cliui.Canceled) {
					return nil
				}
				return err
			}
			var selectedTemplate codersdk.Template
			for _, template := range templates {
				if template.ID == selectedItem {
					selectedTemplate = template
					break
				}
			}

			archive, _, err := client.TemplateArchive(cmd.Context(), selectedTemplate.ID)
			if err != nil {
				return err
			}

			job, parameters, err := createValidProjectVersion(cmd, client, organization, database.ProvisionerType(provisioner), archive)
			if err != nil {
				return err
			}

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Create project?",
				IsConfirm: true,
				Default:   "yes",
			})
			if err != nil {
				if errors.Is(err, promptui.ErrAbort) {
					return nil
				}
				return err
			}

			project, err := client.CreateProject(cmd.Context(), organization.ID, codersdk.CreateProjectRequest{
				Name:            selectedTemplate.ID,
				VersionID:       job.ID,
				ParameterValues: parameters,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s The %s project has been created!\n", caret, color.HiCyanString(project.Name))
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Create a new workspace?",
				IsConfirm: true,
				Default:   "yes",
			})
			if err != nil {
				if errors.Is(err, cliui.Canceled) {
					return nil
				}
				return err
			}

			workspace, err := client.CreateWorkspace(cmd.Context(), "", codersdk.CreateWorkspaceRequest{
				ProjectID: project.ID,
				Name:      selectedTemplate.ID,
			})
			if err != nil {
				return err
			}

			spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
			spin.Writer = cmd.OutOrStdout()
			spin.Suffix = " Building workspace..."
			err = spin.Color("fgHiGreen")
			if err != nil {
				return err
			}
			spin.Start()
			defer spin.Stop()
			logs, err := client.WorkspaceBuildLogsAfter(cmd.Context(), workspace.LatestBuild.ID, time.Time{})
			if err != nil {
				return err
			}
			logBuffer := make([]codersdk.ProvisionerJobLog, 0, 64)
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				logBuffer = append(logBuffer, log)
			}
			build, err := client.WorkspaceBuild(cmd.Context(), workspace.LatestBuild.ID)
			if err != nil {
				return err
			}
			if build.Job.Status != codersdk.ProvisionerJobSucceeded {
				for _, log := range logBuffer {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", color.HiGreenString("[tf]"), log.Output)
				}
				return xerrors.New(build.Job.Error)
			}
			resources, err := client.WorkspaceResourcesByBuild(cmd.Context(), build.ID)
			if err != nil {
				return err
			}
			var workspaceAgent *codersdk.WorkspaceAgent
			for _, resource := range resources {
				if resource.Agent != nil {
					workspaceAgent = resource.Agent
					break
				}
			}
			if workspaceAgent == nil {
				return xerrors.New("something went wrong.. no agent found")
			}
			spin.Suffix = " Waiting for agent to connect..."
			ticker := time.NewTicker(time.Second)
			for {
				select {
				case <-cmd.Context().Done():
					return nil
				case <-ticker.C:
				}
				resource, err := client.WorkspaceResource(cmd.Context(), workspaceAgent.ResourceID)
				if err != nil {
					return err
				}
				if resource.Agent.UpdatedAt.IsZero() {
					continue
				}
				break
			}
			spin.Stop()
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s The %s workspace has been created!\n", caret, color.HiCyanString(project.Name))
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Would you like to SSH?",
				IsConfirm: true,
				Default:   "yes",
			})
			if err != nil {
				if errors.Is(err, cliui.Canceled) {
					return nil
				}
				return err
			}

			dialed, err := client.DialWorkspaceAgent(cmd.Context(), workspaceAgent.ResourceID)
			if err != nil {
				return err
			}
			stream, err := dialed.NegotiateConnection(cmd.Context())
			if err != nil {
				return err
			}
			conn, err := peerbroker.Dial(stream, []webrtc.ICEServer{{
				URLs: []string{"stun:stun.l.google.com:19302"},
			}}, &peer.ConnOptions{})
			if err != nil {
				return err
			}
			sshClient, err := agent.DialSSHClient(conn)
			if err != nil {
				return err
			}
			session, err := sshClient.NewSession()
			if err != nil {
				return err
			}
			state, err := term.MakeRaw(int(os.Stdin.Fd()))
			if err != nil {
				return err
			}
			defer func() {
				_ = term.Restore(int(os.Stdin.Fd()), state)
			}()
			width, height, err := term.GetSize(int(os.Stdin.Fd()))
			if err != nil {
				return err
			}
			err = session.RequestPty("xterm-256color", height, width, ssh.TerminalModes{
				ssh.OCRNL: 1,
			})
			if err != nil {
				return err
			}
			session.Stdin = os.Stdin
			session.Stdout = os.Stdout
			session.Stderr = os.Stderr
			err = session.Shell()
			if err != nil {
				return err
			}
			err = session.Wait()
			if err != nil {
				return err
			}
			return nil
		},
	}
	currentDirectory, _ := os.Getwd()
	cmd.Flags().StringVarP(&directory, "directory", "d", currentDirectory, "Specify the directory to create from")
	cmd.Flags().StringVarP(&provisioner, "provisioner", "p", "terraform", "Customize the provisioner backend")
	// This is for testing!
	err := cmd.Flags().MarkHidden("provisioner")
	if err != nil {
		panic(err)
	}
	return cmd
}

// Show computed parameters for a project version.
// Show computed parameters for a workspace build.
//
// Project Version
//
// Parameters
//   gcp_credentials = us-central1-a (set by workspace)
//    Set with "coder params update org --name test --value something"
//    Description
//     Something about GCP credentials!
//
//    Valid
//      - us-central1-a, us-central1-b, us-central1-c
//
//     x user settable
//     x sensitive
//   region = us-central1-a
//     - user settable
//     - oneof "us-central1-a" "us-central1-b"

//
//   region
//   Description
//     Something about GCP credentials!
//
//
//
// Resources
//   google_compute_instance
//     Shuts off

// Displaying project version information.
// Displaying workspace build information.

func createValidProjectVersion(cmd *cobra.Command, client *codersdk.Client, organization codersdk.Organization, provisioner database.ProvisionerType, archive []byte, parameters ...codersdk.CreateParameterRequest) (*codersdk.ProjectVersion, []codersdk.CreateParameterRequest, error) {
	spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	spin.Writer = cmd.OutOrStdout()
	spin.Suffix = " Uploading current directory..."
	err := spin.Color("fgHiGreen")
	if err != nil {
		return nil, nil, err
	}
	spin.Start()
	defer spin.Stop()

	resp, err := client.Upload(cmd.Context(), codersdk.ContentTypeTar, archive)
	if err != nil {
		return nil, nil, err
	}

	before := time.Now()
	version, err := client.CreateProjectVersion(cmd.Context(), organization.ID, codersdk.CreateProjectVersionRequest{
		StorageMethod:   database.ProvisionerStorageMethodFile,
		StorageSource:   resp.Hash,
		Provisioner:     provisioner,
		ParameterValues: parameters,
	})
	if err != nil {
		return nil, nil, err
	}
	spin.Suffix = " Waiting for the import to complete..."
	logs, err := client.ProjectVersionLogsAfter(cmd.Context(), version.ID, before)
	if err != nil {
		return nil, nil, err
	}
	logBuffer := make([]codersdk.ProvisionerJobLog, 0, 64)
	for {
		log, ok := <-logs
		if !ok {
			break
		}
		logBuffer = append(logBuffer, log)
	}

	version, err = client.ProjectVersion(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	parameterSchemas, err := client.ProjectVersionSchema(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	parameterValues, err := client.ProjectVersionParameters(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	spin.Stop()

	if provisionerd.IsMissingParameterError(version.Job.Error) {
		valuesBySchemaID := map[string]codersdk.ProjectVersionParameter{}
		for _, parameterValue := range parameterValues {
			valuesBySchemaID[parameterValue.SchemaID.String()] = parameterValue
		}
		for _, parameterSchema := range parameterSchemas {
			_, ok := valuesBySchemaID[parameterSchema.ID.String()]
			if ok {
				continue
			}
			value, err := cliui.Prompt(cmd, cliui.PromptOptions{
				Text: fmt.Sprintf("Enter value for %s:", color.HiCyanString(parameterSchema.Name)),
			})
			if err != nil {
				return nil, nil, err
			}
			parameters = append(parameters, codersdk.CreateParameterRequest{
				Name:              parameterSchema.Name,
				SourceValue:       value,
				SourceScheme:      database.ParameterSourceSchemeData,
				DestinationScheme: parameterSchema.DefaultDestinationScheme,
			})
		}
		return createValidProjectVersion(cmd, client, organization, provisioner, archive, parameters...)
	}

	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		for _, log := range logBuffer {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", color.HiGreenString("[tf]"), log.Output)
		}
		return nil, nil, xerrors.New(version.Job.Error)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Successfully imported project source!\n", color.HiGreenString("✓"))

	resources, err := client.ProjectVersionResources(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	return &version, parameters, displayProjectVersionInfo(cmd, parameterSchemas, parameterValues, resources)
}
