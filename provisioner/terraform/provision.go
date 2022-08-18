package terraform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk"
	"github.com/coder/coder/provisionersdk/proto"
)

// Provision executes `terraform apply` or `terraform plan` for dry runs.
func (s *server) Provision(stream proto.DRPCProvisioner_ProvisionStream) error {
	request, err := stream.Recv()
	if err != nil {
		return err
	}
	if request.GetCancel() != nil {
		return nil
	}
	// We expect the first message is start!
	if request.GetStart() == nil {
		return nil
	}

	// Create a context for graceful cancellation bound to the stream
	// context. This ensures that we will perform graceful cancellation
	// even on connection loss.
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// Create a separate context for forcefull cancellation not tied to
	// the stream so that we can control when to terminate the process.
	killCtx, kill := context.WithCancel(context.Background())
	defer kill()

	// Ensure processes are eventually cleaned up on graceful
	// cancellation or disconnect.
	go func() {
		<-stream.Context().Done()

		// TODO(mafredri): We should track this provision request as
		// part of graceful server shutdown procedure. Waiting on a
		// process here should delay provisioner/coder shutdown.
		select {
		case <-time.After(s.exitTimeout):
			kill()
		case <-killCtx.Done():
		}
	}()

	go func() {
		for {
			request, err := stream.Recv()
			if err != nil {
				return
			}
			if request.GetCancel() == nil {
				// We only process cancellation requests here.
				continue
			}
			cancel()
			return
		}
	}()

	logr := streamLogger{stream: stream}
	start := request.GetStart()

	e := s.executor(start.Directory)
	if err = e.checkMinVersion(ctx); err != nil {
		return err
	}
	if err = logTerraformEnvVars(logr); err != nil {
		return err
	}

	statefilePath := filepath.Join(start.Directory, "terraform.tfstate")
	if len(start.State) > 0 {
		err = os.WriteFile(statefilePath, start.State, 0o600)
		if err != nil {
			return xerrors.Errorf("write statefile %q: %w", statefilePath, err)
		}
	}

	// If we're destroying, exit early if there's no state. This is necessary to
	// avoid any cases where a workspace is "locked out" of terraform due to
	// e.g. bad template param values and cannot be deleted. This is just for
	// contingency, in the future we will try harder to prevent workspaces being
	// broken this hard.
	if start.Metadata.WorkspaceTransition == proto.WorkspaceTransition_DESTROY && len(start.State) == 0 {
		_ = stream.Send(&proto.Provision_Response{
			Type: &proto.Provision_Response_Log{
				Log: &proto.Log{
					Level:  proto.LogLevel_INFO,
					Output: "The terraform state does not exist, there is nothing to do",
				},
			},
		})

		return stream.Send(&proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{},
			},
		})
	}

	s.logger.Debug(ctx, "running initialization")
	err = e.init(ctx, killCtx, logr)
	if err != nil {
		if ctx.Err() != nil {
			return stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Complete{
					Complete: &proto.Provision_Complete{
						Error: err.Error(),
					},
				},
			})
		}
		return xerrors.Errorf("initialize terraform: %w", err)
	}
	s.logger.Debug(ctx, "ran initialization")

	env, err := provisionEnv(start)
	if err != nil {
		return err
	}
	vars, err := provisionVars(start)
	if err != nil {
		return err
	}
	var resp *proto.Provision_Response
	if start.DryRun {
		resp, err = e.plan(ctx, killCtx, env, vars, logr,
			start.Metadata.WorkspaceTransition == proto.WorkspaceTransition_DESTROY)
	} else {
		resp, err = e.apply(ctx, killCtx, env, vars, logr,
			start.Metadata.WorkspaceTransition == proto.WorkspaceTransition_DESTROY)
	}
	if err != nil {
		if start.DryRun {
			if ctx.Err() != nil {
				return stream.Send(&proto.Provision_Response{
					Type: &proto.Provision_Response_Complete{
						Complete: &proto.Provision_Complete{
							Error: err.Error(),
						},
					},
				})
			}
			return xerrors.Errorf("plan terraform: %w", err)
		}
		errorMessage := err.Error()
		// Terraform can fail and apply and still need to store it's state.
		// In this case, we return Complete with an explicit error message.
		stateData, _ := os.ReadFile(statefilePath)
		return stream.Send(&proto.Provision_Response{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					State: stateData,
					Error: errorMessage,
				},
			},
		})
	}

	return stream.Send(resp)
}

func provisionVars(start *proto.Provision_Start) ([]string, error) {
	vars := []string{}
	for _, param := range start.ParameterValues {
		switch param.DestinationScheme {
		case proto.ParameterDestination_ENVIRONMENT_VARIABLE:
			continue
		case proto.ParameterDestination_PROVISIONER_VARIABLE:
			vars = append(vars, fmt.Sprintf("%s=%s", param.Name, param.Value))
		default:
			return nil, xerrors.Errorf("unsupported parameter type %q for %q", param.DestinationScheme, param.Name)
		}
	}
	return vars, nil
}

func provisionEnv(start *proto.Provision_Start) ([]string, error) {
	env := os.Environ()
	env = append(env,
		"CODER_AGENT_URL="+start.Metadata.CoderUrl,
		"CODER_WORKSPACE_TRANSITION="+strings.ToLower(start.Metadata.WorkspaceTransition.String()),
		"CODER_WORKSPACE_NAME="+start.Metadata.WorkspaceName,
		"CODER_WORKSPACE_OWNER="+start.Metadata.WorkspaceOwner,
		"CODER_WORKSPACE_OWNER_EMAIL="+start.Metadata.WorkspaceOwnerEmail,
		"CODER_WORKSPACE_ID="+start.Metadata.WorkspaceId,
		"CODER_WORKSPACE_OWNER_ID="+start.Metadata.WorkspaceOwnerId,
	)
	for key, value := range provisionersdk.AgentScriptEnv() {
		env = append(env, key+"="+value)
	}
	for _, param := range start.ParameterValues {
		switch param.DestinationScheme {
		case proto.ParameterDestination_ENVIRONMENT_VARIABLE:
			env = append(env, fmt.Sprintf("%s=%s", param.Name, param.Value))
		case proto.ParameterDestination_PROVISIONER_VARIABLE:
			continue
		default:
			return nil, xerrors.Errorf("unsupported parameter type %q for %q", param.DestinationScheme, param.Name)
		}
	}
	return env, nil
}

var (
	// tfEnvSafeToPrint is the set of terraform environment variables that we are quite sure won't contain secrets,
	// and therefore it's ok to log their values
	tfEnvSafeToPrint = map[string]bool{
		"TF_LOG":                      true,
		"TF_LOG_PATH":                 true,
		"TF_INPUT":                    true,
		"TF_DATA_DIR":                 true,
		"TF_WORKSPACE":                true,
		"TF_IN_AUTOMATION":            true,
		"TF_REGISTRY_DISCOVERY_RETRY": true,
		"TF_REGISTRY_CLIENT_TIMEOUT":  true,
		"TF_CLI_CONFIG_FILE":          true,
		"TF_IGNORE":                   true,
	}
)

func logTerraformEnvVars(logr logger) error {
	env := os.Environ()
	for _, e := range env {
		if strings.HasPrefix(e, "TF_") {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) != 2 {
				panic("os.Environ() returned vars not in key=value form")
			}
			if !tfEnvSafeToPrint[parts[0]] {
				parts[1] = "<value redacted>"
			}
			err := logr.Log(&proto.Log{
				Level:  proto.LogLevel_WARN,
				Output: fmt.Sprintf("terraform environment variable: %s=%s", parts[0], parts[1]),
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
