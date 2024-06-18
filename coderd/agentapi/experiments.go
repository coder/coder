package agentapi

import (
	"context"

	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

type ExperimentAPI struct {
	experiments codersdk.Experiments
}

func (a *ExperimentAPI) GetExperiments(_ context.Context, _ *proto.GetExperimentsRequest) (*proto.GetExperimentsResponse, error) {
	return agentsdk.ProtoFromExperiments(a.experiments), nil
}
