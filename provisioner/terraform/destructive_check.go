package terraform

import (
	"fmt"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
)

// persistentResourceTypes lists resource types that typically hold
// persistent user data. A plan that destroys or replaces one of
// these is blocked unless the workspace transition is "destroy."
//
// This is intentionally hardcoded for now. A future iteration
// could let template authors tag resources via coder_metadata.
var persistentResourceTypes = []string{
	// Azure
	"azurerm_managed_disk",
	// AWS
	"aws_ebs_volume",
	// GCP
	"google_compute_disk",
	// Kubernetes
	"kubernetes_persistent_volume_claim",
	// Docker
	"docker_volume",
}

// ConfirmDestroyParamName is the parameter name the frontend
// injects when the user confirms a destructive change.
const ConfirmDestroyParamName = "__confirm_persistent_resource_destruction__"

// hasConfirmDestroyParam checks the build parameters for the
// confirmation bypass.
func hasConfirmDestroyParam(params []*sdkproto.RichParameterValue) bool {
	for _, p := range params {
		if p.GetName() == ConfirmDestroyParamName && p.GetValue() == "true" {
			return true
		}
	}
	return false
}

// checkDestructiveChanges inspects the plan for any destroy or
// replace action on a persistent resource type. If found, it
// returns an error describing which resources are at risk. The
// error message is shown to the user in the build logs.
func checkDestructiveChanges(plan *tfjson.Plan) error {
	if plan == nil {
		return nil
	}

	var affected []string
	for _, ch := range plan.ResourceChanges {
		if ch.Change == nil {
			continue
		}
		if !ch.Change.Actions.Delete() && !ch.Change.Actions.Replace() {
			continue
		}
		if !isPersistentResource(ch.Type) {
			continue
		}
		action := "destroyed"
		if ch.Change.Actions.Replace() {
			action = "replaced (destroyed and recreated)"
		}
		affected = append(affected, fmt.Sprintf(
			"  %s will be %s", ch.Address, action))
	}

	if len(affected) == 0 {
		return nil
	}

	return fmt.Errorf(strings.Join(append([]string{
		"CODER_BLOCK_DESTROY:",
		"This update will destroy persistent resources containing user data:",
		"",
	}, append(affected,
		"",
		"All data on these resources will be permanently lost.",
	)...,
	), "\n"))
}

func isPersistentResource(resourceType string) bool {
	for _, t := range persistentResourceTypes {
		if resourceType == t {
			return true
		}
	}
	return false
}
