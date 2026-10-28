//go:build linux || darwin

package terraform_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfstate"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateResourcesPartialFailureTerraform(t *testing.T) {
	t.Parallel()
	for _, destroy := range []bool{false, true} {
		t.Run(fmt.Sprintf("Destroy=%t", destroy), func(t *testing.T) {
			t.Parallel()
			when := "create"
			if destroy {
				when = "destroy"
			}
			source := fmt.Sprintf(`resource "terraform_data" "failed" {
  provisioner "local-exec" {
    when = %s
    command = "exit 1"
  }
}
resource "terraform_data" "other" {
  input = "preserved partial state"
}`, when)
			_, client := setupProvisioner(t, nil)
			ctx := testutil.Context(t, testutil.WaitLong)
			sess := configure(ctx, t, client, &proto.Config{})
			defer sess.Close()
			init := sendInitAndGetResp(t, sess, testutil.CreateTar(t, map[string]string{"main.tf": source}))
			require.Empty(t, init.Error)
			require.NoError(t, sendPlan(sess, proto.WorkspaceTransition_START))
			_, response := readProvisionLog(t, sess)
			require.NotNil(t, response.GetPlan())
			require.Empty(t, response.GetPlan().Error)
			require.NoError(t, sendApply(sess, proto.WorkspaceTransition_START))
			_, response = readProvisionLog(t, sess)
			require.NotNil(t, response.GetApply())
			if destroy {
				require.Empty(t, response.GetApply().Error)
				require.NoError(t, sess.Send(&proto.Request{Type: &proto.Request_Plan{Plan: &proto.PlanRequest{
					Metadata: &proto.Metadata{WorkspaceTransition: proto.WorkspaceTransition_DESTROY},
					State:    response.GetApply().State,
				}}}))
				_, response = readProvisionLog(t, sess)
				require.NotNil(t, response.GetPlan())
				require.Empty(t, response.GetPlan().Error)
				require.NoError(t, sendApply(sess, proto.WorkspaceTransition_DESTROY))
				_, response = readProvisionLog(t, sess)
			}
			apply := response.GetApply()
			require.NotNil(t, apply)
			require.NotEmpty(t, apply.Error)
			require.True(t, apply.StateIsAuthoritative)
			require.NoError(t, tfstate.ValidateState(apply.State))
			require.Error(t, tfstate.ValidateDestroyState(apply.State), "failed mutation must retain managed resources")
		})
	}
}

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
			require.True(t, response.GetApply().StateIsAuthoritative)
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
			require.True(t, response.GetApply().StateIsAuthoritative)
			require.NoError(t, tfstate.ValidateDestroyState(response.GetApply().State))
		})
	}
}
