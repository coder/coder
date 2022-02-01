package terraform

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-exec/tfexec"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk/proto"
)

// Provision executes `terraform apply`.
func (t *terraform) Provision(request *proto.Provision_Request, stream proto.DRPCProvisioner_ProvisionStream) error {
	ctx := stream.Context()
	statefilePath := filepath.Join(request.Directory, "terraform.tfstate")
	if len(request.State) > 0 {
		err := os.WriteFile(statefilePath, request.State, 0600)
		if err != nil {
			return xerrors.Errorf("write statefile %q: %w", statefilePath, err)
		}
	}

	terraform, err := tfexec.NewTerraform(request.Directory, t.binaryPath)
	if err != nil {
		return xerrors.Errorf("create new terraform executor: %w", err)
	}
	version, _, err := terraform.Version(ctx, false)
	if err != nil {
		return xerrors.Errorf("get terraform version: %w", err)
	}
	if !version.GreaterThanOrEqual(minimumTerraformVersion) {
		return xerrors.Errorf("terraform version %q is too old. required >= %q", version.String(), minimumTerraformVersion.String())
	}

	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			_ = stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: scanner.Text(),
					},
				},
			})
		}
	}()
	terraform.SetStdout(writer)
	err = terraform.Init(ctx)
	if err != nil {
		return xerrors.Errorf("initialize terraform: %w", err)
	}

	env := map[string]string{}
	options := []tfexec.ApplyOption{tfexec.JSON(true)}
	for _, param := range request.ParameterValues {
		switch param.DestinationScheme {
		case proto.ParameterDestination_ENVIRONMENT_VARIABLE:
			env[param.Name] = param.Value
		case proto.ParameterDestination_PROVISIONER_VARIABLE:
			options = append(options, tfexec.Var(fmt.Sprintf("%s=%s", param.Name, param.Value)))
		default:
			return xerrors.Errorf("unsupported parameter type %q for %q", param.DestinationScheme, param.Name)
		}
	}
	err = terraform.SetEnv(env)
	if err != nil {
		return xerrors.Errorf("apply environment variables: %w", err)
	}

	reader, writer = io.Pipe()
	defer reader.Close()
	defer writer.Close()
	go func() {
		decoder := json.NewDecoder(reader)
		for {
			var log terraformProvisionLog
			err := decoder.Decode(&log)
			if err != nil {
				return
			}

			logLevel, err := convertTerraformLogLevel(log.Level)
			if err != nil {
				// Not a big deal, but we should handle this at some point!
				continue
			}
			_ = stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  logLevel,
						Output: log.Message,
					},
				},
			})

			if log.Diagnostic == nil {
				continue
			}

			// If the diagnostic is provided, let's provide a bit more info!
			logLevel, err = convertTerraformLogLevel(log.Diagnostic.Severity)
			if err != nil {
				continue
			}
			_ = stream.Send(&proto.Provision_Response{
				Type: &proto.Provision_Response_Log{
					Log: &proto.Log{
						Level:  logLevel,
						Output: log.Diagnostic.Detail,
					},
				},
			})
		}
	}()

	terraform.SetStdout(writer)
	err = terraform.Apply(ctx, options...)
	if err != nil {
		return xerrors.Errorf("apply terraform: %w", err)
	}

	statefileContent, err := os.ReadFile(statefilePath)
	if err != nil {
		return xerrors.Errorf("read file %q: %w", statefilePath, err)
	}
	state, err := terraform.ShowStateFile(ctx, statefilePath)
	if err != nil {
		return xerrors.Errorf("show state file %q: %w", statefilePath, err)
	}
	resources := make([]*proto.Resource, 0)
	if state.Values != nil {
		for _, resource := range state.Values.RootModule.Resources {
			resources = append(resources, &proto.Resource{
				Name: resource.Name,
				Type: resource.Type,
			})
		}
	}

	return stream.Send(&proto.Provision_Response{
		Type: &proto.Provision_Response_Complete{
			Complete: &proto.Provision_Complete{
				State:     statefileContent,
				Resources: resources,
			},
		},
	})
}

type terraformProvisionLog struct {
	Level   string `json:"@level"`
	Message string `json:"@message"`

	Diagnostic *terraformProvisionLogDiagnostic `json:"diagnostic"`
}

type terraformProvisionLogDiagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
}

func convertTerraformLogLevel(logLevel string) (proto.LogLevel, error) {
	switch strings.ToLower(logLevel) {
	case "trace":
		return proto.LogLevel_TRACE, nil
	case "debug":
		return proto.LogLevel_DEBUG, nil
	case "info":
		return proto.LogLevel_INFO, nil
	case "warn":
		return proto.LogLevel_WARN, nil
	case "error":
		return proto.LogLevel_ERROR, nil
	default:
		return proto.LogLevel(0), xerrors.Errorf("invalid log level %q", logLevel)
	}
}
