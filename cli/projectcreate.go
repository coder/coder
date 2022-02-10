package cli

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
)

func projectCreate() *cobra.Command {
	var (
		directory string
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
			_, err = runPrompt(cmd, &promptui.Prompt{
				Default:   "y",
				IsConfirm: true,
				Label:     fmt.Sprintf("Set up %s in your organization?", color.New(color.FgHiCyan).Sprintf("%q", directory)),
			})
			if err != nil {
				return err
			}

			name, err := runPrompt(cmd, &promptui.Prompt{
				Default: filepath.Base(directory),
				Label:   "What's your project's name?",
				Validate: func(s string) error {
					_, err = client.Project(cmd.Context(), organization.Name, s)
					if err == nil {
						return xerrors.New("A project already exists with that name!")
					}
					return nil
				},
			})
			if err != nil {
				return err
			}

			spin := spinner.New(spinner.CharSets[0], 25*time.Millisecond)
			spin.Suffix = " Uploading current directory..."
			spin.Start()
			defer spin.Stop()

			bytes, err := tarDirectory(directory)
			if err != nil {
				return err
			}

			resp, err := client.UploadFile(cmd.Context(), codersdk.ContentTypeTar, bytes)
			if err != nil {
				return err
			}

			job, err := client.CreateProjectVersionImportProvisionerJob(cmd.Context(), organization.Name, coderd.CreateProjectImportJobRequest{
				StorageMethod: database.ProvisionerStorageMethodFile,
				StorageSource: resp.Hash,
				Provisioner:   database.ProvisionerTypeTerraform,
				// SkipResources on first import to detect variables defined by the project.
				SkipResources: true,
				// ParameterValues: []coderd.CreateParameterValueRequest{{
				// 	Name:              "aws_access_key",
				// 	SourceValue:       "tomato",
				// 	SourceScheme:      database.ParameterSourceSchemeData,
				// 	DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
				// }},
			})
			if err != nil {
				return err
			}
			spin.Stop()

			logs, err := client.FollowProvisionerJobLogsAfter(cmd.Context(), organization.Name, job.ID, time.Time{})
			if err != nil {
				return err
			}
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", color.HiGreenString("[parse]"), log.Output)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Parsed project source... displaying parameters:")

			schemas, err := client.ProvisionerJobParameterSchemas(cmd.Context(), organization.Name, job.ID)
			if err != nil {
				return err
			}

			values, err := client.ProvisionerJobParameterValues(cmd.Context(), organization.Name, job.ID)
			if err != nil {
				return err
			}
			valueBySchemaID := map[string]coderd.ComputedParameterValue{}
			for _, value := range values {
				valueBySchemaID[value.SchemaID.String()] = value
			}

			for _, schema := range schemas {
				if value, ok := valueBySchemaID[schema.ID.String()]; ok {
					fmt.Printf("Value for: %s %s\n", value.Name, value.SourceValue)
					continue
				}
				fmt.Printf("No value for: %s\n", schema.Name)
			}

			// schemas, err := client.ProvisionerJobParameterSchemas(cmd.Context(), organization.Name, job.ID)
			// if err != nil {
			// 	return err
			// }
			// _, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n  %s\n\n", color.HiBlackString("Parameters"))

			// for _, param := range params {
			// 	if param.Value == nil {
			// 		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    %s = must be set\n", color.HiRedString(param.Schema.Name))
			// 		continue
			// 	}
			// 	value := param.Value.DestinationValue
			// 	if !param.Schema.RedisplayValue {
			// 		value = "<redacted>"
			// 	}
			// 	output := fmt.Sprintf("    %s = %s", color.HiGreenString(param.Value.SourceValue), color.CyanString(value))
			// 	param.Value.DefaultSourceValue = false
			// 	param.Value.Scope = database.ParameterScopeOrganization
			// 	param.Value.ScopeID = organization.ID
			// 	if param.Value.DefaultSourceValue {
			// 		output += " (default value)"
			// 	} else {
			// 		output += fmt.Sprintf(" (inherited from %s)", param.Value.Scope)
			// 	}
			// 	root := treeprint.NewWithRoot(output)
			// 	root.AddNode(color.HiBlackString("Description") + "\n" + param.Schema.Description)
			// 	fmt.Fprintln(cmd.OutOrStdout(), strings.Join(strings.Split(root.String(), "\n"), "\n    "))
			// }

			// for _, param := range params {
			// 	if param.Value != nil {
			// 		continue
			// 	}

			// 	value, err := runPrompt(cmd, &promptui.Prompt{
			// 		Label: "Specify value for " + color.HiCyanString(param.Schema.Name),
			// 		Validate: func(s string) error {
			// 			// param.Schema.Vali
			// 			return nil
			// 		},
			// 	})
			// 	if err != nil {
			// 		continue
			// 	}
			// 	fmt.Printf(": %s\n", value)
			// }

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Create project %q!\n", name)
			return nil
		},
	}
	currentDirectory, _ := os.Getwd()
	cmd.Flags().StringVarP(&directory, "directory", "d", currentDirectory, "Specify the directory to create from")

	return cmd
}

func tarDirectory(directory string) ([]byte, error) {
	var buffer bytes.Buffer
	tarWriter := tar.NewWriter(&buffer)
	err := filepath.Walk(directory, func(file string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(fileInfo, file)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(directory, file)
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if fileInfo.IsDir() {
			return nil
		}
		data, err := os.Open(file)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tarWriter, data); err != nil {
			return err
		}
		return data.Close()
	})
	if err != nil {
		return nil, err
	}
	err = tarWriter.Flush()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
