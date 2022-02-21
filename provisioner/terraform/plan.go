package terraform

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-exec/tfexec"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk/proto"
)

// Provision executes `terraform apply`.
func (t *terraform) Plan(request *proto.Plan_Request, stream proto.DRPCProvisioner_PlanStream) error {
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
			_ = stream.Send(&proto.Plan_Response{
				Type: &proto.Plan_Response_Log{
					Log: &proto.Log{
						Level:  proto.LogLevel_INFO,
						Output: scanner.Text(),
					},
				},
			})
		}
	}()
	terraform.SetStdout(writer)
	t.logger.Debug(ctx, "running initialization")
	err = terraform.Init(ctx)
	if err != nil {
		return xerrors.Errorf("initialize terraform: %w", err)
	}
	t.logger.Debug(ctx, "ran initialization")

	env := map[string]string{}
	planfilePath := filepath.Join(request.Directory, "terraform.tfplan")
	options := []tfexec.PlanOption{tfexec.JSON(true), tfexec.Out(planfilePath)}
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

	resources := make([]*proto.PlannedResource, 0)
	reader, writer = io.Pipe()
	defer reader.Close()
	defer writer.Close()
	closeChan := make(chan struct{})
	go func() {
		defer close(closeChan)
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
			_ = stream.Send(&proto.Plan_Response{
				Type: &proto.Plan_Response_Log{
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
			_ = stream.Send(&proto.Plan_Response{
				Type: &proto.Plan_Response_Log{
					Log: &proto.Log{
						Level:  logLevel,
						Output: log.Diagnostic.Detail,
					},
				},
			})
		}
	}()

	terraform.SetStdout(writer)
	t.logger.Debug(ctx, "running plan")
	_, err = terraform.Plan(ctx, options...)
	if err != nil {
		return xerrors.Errorf("apply terraform: %w", err)
	}
	t.logger.Debug(ctx, "ran plan")

	plan, err := terraform.ShowPlanFile(ctx, planfilePath)
	if err != nil {
		return xerrors.Errorf("show plan file: %w", err)
	}
	_ = reader.Close()
	<-closeChan

	for _, resource := range plan.ResourceChanges {
		protoResource := &proto.PlannedResource{
			Name: resource.Name,
			Type: resource.Type,
		}
		afterUnknownMap, ok := resource.Change.AfterUnknown.(map[string]interface{})
		if ok {
			// This is the specific key used by the Terraform Google Cloud Provisioner
			// to identify an instance.
			if _, hasGoogleCloudInstanceID := afterUnknownMap["instance_id"]; hasGoogleCloudInstanceID {
				protoResource.Agent = true
			}
		}
		resources = append(resources, protoResource)
	}

	return stream.Send(&proto.Plan_Response{
		Type: &proto.Plan_Response_Complete{
			Complete: &proto.Plan_Complete{
				Resources: resources,
			},
		},
	})
}
