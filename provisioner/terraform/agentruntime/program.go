package agentruntime

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/xerrors"
)

// Program holds request-local Terraform configuration and provenance indexes
// for agent-runtime resolution.
type Program struct {
	configIndex *configIndex
}

// NewProgram indexes Terraform configuration for agent-runtime resolution.
func NewProgram(
	ctx context.Context,
	config *tfjson.Config,
) (*Program, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	configIndex, err := newConfigIndex(ctx, config)
	if err != nil {
		return nil, err
	}
	if configIndex == nil {
		return nil, xerrors.New("Terraform plan configuration is unavailable")
	}
	return &Program{configIndex: configIndex}, nil
}

// LoadProvenance indexes agent-runtime expressions from Terraform sources in
// workdir.
func (p *Program) LoadProvenance(ctx context.Context, workdir string) error {
	if p == nil || p.configIndex == nil {
		return xerrors.New("Terraform plan configuration is unavailable")
	}
	return p.configIndex.indexRuntimeSourceExpressions(ctx, workdir)
}
