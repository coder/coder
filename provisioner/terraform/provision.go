package terraform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-exec/tfexec"
	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk/proto"
)

// Provision executes `terraform apply`.
func (t *terraform) Provision(ctx context.Context, request *proto.Provision_Request) (*proto.Provision_Response, error) {
	statefilePath := filepath.Join(request.Directory, "terraform.tfstate")
	err := os.WriteFile(statefilePath, request.State, 0600)
	if err != nil {
		return nil, xerrors.Errorf("write statefile %q: %w", statefilePath, err)
	}

	terraform, err := tfexec.NewTerraform(request.Directory, t.binaryPath)
	if err != nil {
		return nil, xerrors.Errorf("create new terraform executor: %w", err)
	}
	version, _, err := terraform.Version(ctx, false)
	if err != nil {
		return nil, xerrors.Errorf("get terraform version: %w", err)
	}
	if !version.GreaterThanOrEqual(minimumTerraformVersion) {
		return nil, xerrors.Errorf("terraform version %q is too old. required >= %q", version.String(), minimumTerraformVersion.String())
	}

	err = terraform.Init(ctx)
	if err != nil {
		return nil, xerrors.Errorf("initialize terraform: %w", err)
	}

	options := make([]tfexec.ApplyOption, 0)
	for _, params := range request.ParameterValues {
		options = append(options, tfexec.Var(fmt.Sprintf("%s=%s", params.Name, params.Value)))
	}
	err = terraform.Apply(ctx, options...)
	if err != nil {
		return nil, xerrors.Errorf("apply terraform: %w", err)
	}

	statefileContent, err := os.ReadFile(statefilePath)
	if err != nil {
		return nil, xerrors.Errorf("read file %q: %w", statefilePath, err)
	}
	state, err := terraform.ShowStateFile(ctx, statefilePath)
	if err != nil {
		return nil, xerrors.Errorf("show state file %q: %w", statefilePath, err)
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

	return &proto.Provision_Response{
		Resources: resources,
		State:     statefileContent,
	}, nil
}
