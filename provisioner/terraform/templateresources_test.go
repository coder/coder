//go:build linux || darwin

package terraform_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfstate"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateResourcesStandaloneTerraform(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source string }{
		{"OutputsOnly", `variable "value" { type = string }
output "shared" { value = { nested = [var.value] } }`},
		{"ManagedResource", `variable "value" { type = string }
resource "terraform_data" "shared" { input = var.value }
output "shared" { value = terraform_data.shared.output }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, client := setupProvisioner(t, nil)
			ctx := testutil.Context(t, testutil.WaitLong)
			sess := configure(ctx, t, client, &proto.Config{})
			defer sess.Close()
			init := sendInitAndGetResp(t, sess, testutil.CreateTar(t, map[string]string{"main.tf": tc.source}))
			require.Empty(t, init.Error)
			metadata := &proto.Metadata{WorkspaceTransition: proto.WorkspaceTransition_START}
			variables := []*proto.VariableValue{{Name: "value", Value: "arbitrary input"}}
			require.NoError(t, sess.Send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
				Metadata: metadata, VariableValues: variables,
			}}}))
			_, response := readProvisionLog(t, sess)
			require.NotNil(t, response.GetPlan())
			require.Empty(t, response.GetPlan().Error)
			require.NoError(t, sess.Send(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{Metadata: metadata}}}))
			_, response = readProvisionLog(t, sess)
			require.NotNil(t, response.GetApply())
			require.Empty(t, response.GetApply().Error)
			require.NoError(t, tfstate.ValidateState(response.GetApply().State))
			state := response.GetApply().State

			metadata.WorkspaceTransition = proto.WorkspaceTransition_DESTROY
			require.NoError(t, sess.Send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
				Metadata: metadata, VariableValues: variables, State: state,
			}}}))
			_, response = readProvisionLog(t, sess)
			require.NotNil(t, response.GetPlan())
			require.Empty(t, response.GetPlan().Error)
			require.NoError(t, sess.Send(&proto.Request{Type: &proto.Request_Apply{Apply: &proto.ApplyRequest{Metadata: metadata}}}))
			_, response = readProvisionLog(t, sess)
			require.NotNil(t, response.GetApply())
			require.Empty(t, response.GetApply().Error)
			require.NoError(t, tfstate.ValidateDestroyState(response.GetApply().State))
		})
	}
}
