// Package tfstate validates Terraform state snapshots used by native jobs.
package tfstate

import (
	"encoding/json"

	"golang.org/x/xerrors"
)

// ValidateNetworkPolicyDestroyState requires a Terraform v4 state snapshot with
// no remaining managed instances, including deposed instances. A plan preview
// or a missing state file is not evidence that infrastructure was destroyed.
func ValidateNetworkPolicyDestroyState(data []byte) error {
	var state struct {
		Version          int                        `json:"version"`
		TerraformVersion string                     `json:"terraform_version"`
		Serial           *int64                     `json:"serial"`
		Lineage          string                     `json:"lineage"`
		Outputs          map[string]json.RawMessage `json:"outputs"`
		Resources        json.RawMessage            `json:"resources"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return xerrors.Errorf("decode Terraform destroy state: %w", err)
	}
	if state.Version != 4 || state.TerraformVersion == "" || state.Serial == nil || *state.Serial < 0 || state.Lineage == "" || len(state.Resources) == 0 || string(state.Resources) == "null" {
		return xerrors.New("destroy requires a complete Terraform v4 state snapshot")
	}
	var resources []struct {
		Mode      string            `json:"mode"`
		Instances []json.RawMessage `json:"instances"`
	}
	if err := json.Unmarshal(state.Resources, &resources); err != nil {
		return xerrors.Errorf("decode Terraform destroy resources: %w", err)
	}
	for _, resource := range resources {
		switch resource.Mode {
		case "managed":
			if len(resource.Instances) != 0 {
				return xerrors.New("destroy state still contains managed instances")
			}
		case "data":
		default:
			return xerrors.New("destroy state contains an invalid resource mode")
		}
	}
	return nil
}
