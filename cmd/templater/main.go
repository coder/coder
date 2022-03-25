package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/tunnel"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/terraform"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

func main() {
	var rawParameters []string
	cmd := &cobra.Command{
		Use: "templater",
		RunE: func(cmd *cobra.Command, args []string) error {
			parameters := make([]codersdk.CreateParameterRequest, 0)
			for _, parameter := range rawParameters {
				parts := strings.SplitN(parameter, "=", 2)
				parameters = append(parameters, codersdk.CreateParameterRequest{
					Name:              parts[0],
					SourceValue:       parts[1],
					SourceScheme:      database.ParameterSourceSchemeData,
					DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
				})
			}
			return parse(cmd, parameters)
		},
	}
	cmd.Flags().StringArrayVarP(&rawParameters, "parameter", "p", []string{}, "Specify parameters to pass in a template.")
	err := cmd.Execute()
	if err != nil {
		panic(err)
	}
}

func parse(cmd *cobra.Command, parameters []codersdk.CreateParameterRequest) error {
	srv := httptest.NewUnstartedServer(nil)
	srv.Config.BaseContext = func(_ net.Listener) context.Context {
		return cmd.Context()
	}
	srv.Start()
	serverURL, err := url.Parse(srv.URL)
	if err != nil {
		return err
	}
	accessURL, errCh, err := tunnel.New(cmd.Context(), srv.URL)
	if err != nil {
		return err
	}
	go func() {
		err := <-errCh
		if err != nil {
			panic(err)
		}
	}()
	accessURLParsed, err := url.Parse(accessURL)
	if err != nil {
		return err
	}
	var closeWait func()
	validator, err := idtoken.NewValidator(cmd.Context())
	if err != nil {
		return err
	}
	logger := slog.Make(sloghuman.Sink(cmd.OutOrStdout()))
	srv.Config.Handler, closeWait = coderd.New(&coderd.Options{
		AccessURL:            accessURLParsed,
		Logger:               logger,
		Database:             databasefake.New(),
		Pubsub:               database.NewPubsubInMemory(),
		GoogleTokenValidator: validator,
	})

	client := codersdk.New(serverURL)
	daemonClose, err := newProvisionerDaemon(cmd.Context(), client, logger)
	if err != nil {
		return err
	}
	defer daemonClose.Close()

	created, err := client.CreateFirstUser(cmd.Context(), codersdk.CreateFirstUserRequest{
		Email:        "templater@coder.com",
		Username:     "templater",
		Organization: "templater",
		Password:     "insecure",
	})
	if err != nil {
		return err
	}
	auth, err := client.LoginWithPassword(cmd.Context(), codersdk.LoginWithPasswordRequest{
		Email:    "templater@coder.com",
		Password: "insecure",
	})
	if err != nil {
		return err
	}
	client.SessionToken = auth.SessionToken

	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	content, err := provisionersdk.Tar(dir)
	if err != nil {
		return err
	}
	resp, err := client.Upload(cmd.Context(), codersdk.ContentTypeTar, content)
	if err != nil {
		return err
	}

	before := time.Now()
	version, err := client.CreateProjectVersion(cmd.Context(), created.OrganizationID, codersdk.CreateProjectVersionRequest{
		StorageMethod:   database.ProvisionerStorageMethodFile,
		StorageSource:   resp.Hash,
		Provisioner:     database.ProvisionerTypeTerraform,
		ParameterValues: parameters,
	})
	if err != nil {
		return err
	}
	logs, err := client.ProjectVersionLogsAfter(cmd.Context(), version.ID, before)
	if err != nil {
		return err
	}
	for {
		log, ok := <-logs
		if !ok {
			break
		}
		_, _ = fmt.Printf("terraform (%s): %s\n", log.Level, log.Output)
	}
	version, err = client.ProjectVersion(cmd.Context(), version.ID)
	if err != nil {
		return err
	}
	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		return xerrors.Errorf("Job wasn't successful, it was %q. Check the logs!", version.Job.Status)
	}

	_, err = client.ProjectVersionResources(cmd.Context(), version.ID)
	if err != nil {
		return err
	}

	project, err := client.CreateProject(cmd.Context(), created.OrganizationID, codersdk.CreateProjectRequest{
		Name:      "test",
		VersionID: version.ID,
	})
	if err != nil {
		return err
	}

	workspace, err := client.CreateWorkspace(cmd.Context(), created.UserID, codersdk.CreateWorkspaceRequest{
		ProjectID: project.ID,
		Name:      "example",
	})
	if err != nil {
		return err
	}
	logs, err = client.WorkspaceBuildLogsAfter(cmd.Context(), workspace.LatestBuild.ID, before)
	if err != nil {
		return err
	}
	for {
		log, ok := <-logs
		if !ok {
			break
		}
		_, _ = fmt.Printf("terraform (%s): %s\n", log.Level, log.Output)
	}

	resources, err := client.WorkspaceResourcesByBuild(cmd.Context(), workspace.LatestBuild.ID)
	if err != nil {
		return err
	}
	for _, resource := range resources {
		if resource.Agent == nil {
			continue
		}
		err = awaitAgent(cmd.Context(), client, resource)
		if err != nil {
			return err
		}
	}

	build, err := client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
		ProjectVersionID: version.ID,
		Transition:       database.WorkspaceTransitionDelete,
	})
	if err != nil {
		return err
	}
	logs, err = client.WorkspaceBuildLogsAfter(cmd.Context(), build.ID, before)
	if err != nil {
		return err
	}
	for {
		log, ok := <-logs
		if !ok {
			break
		}
		_, _ = fmt.Printf("terraform (%s): %s\n", log.Level, log.Output)
	}

	_ = daemonClose.Close()
	srv.Close()
	closeWait()
	return nil
}

func awaitAgent(ctx context.Context, client *codersdk.Client, resource codersdk.WorkspaceResource) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resource, err := client.WorkspaceResource(ctx, resource.ID)
			if err != nil {
				return err
			}
			if resource.Agent.FirstConnectedAt == nil {
				continue
			}
			return nil
		}
	}
}

func newProvisionerDaemon(ctx context.Context, client *codersdk.Client, logger slog.Logger) (io.Closer, error) {
	terraformClient, terraformServer := provisionersdk.TransportPipe()
	go func() {
		err := terraform.Serve(ctx, &terraform.ServeOptions{
			ServeOptions: &provisionersdk.ServeOptions{
				Listener: terraformServer,
			},
			Logger: logger,
		})
		if err != nil {
			panic(err)
		}
	}()
	tempDir, err := ioutil.TempDir("", "provisionerd")
	if err != nil {
		return nil, err
	}
	return provisionerd.New(client.ListenProvisionerDaemon, &provisionerd.Options{
		Logger:         logger,
		PollInterval:   50 * time.Millisecond,
		UpdateInterval: 500 * time.Millisecond,
		Provisioners: provisionerd.Provisioners{
			string(database.ProvisionerTypeTerraform): proto.NewDRPCProvisionerClient(provisionersdk.Conn(terraformClient)),
		},
		WorkDirectory: tempDir,
	}), nil
}
