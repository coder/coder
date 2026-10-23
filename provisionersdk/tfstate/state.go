package tfstate

import (
	"encoding/json"

	"golang.org/x/xerrors"
)

// ValidateState requires a complete Terraform v4 state snapshot. Outputs are
// decoded and bounded by the server before they can be exposed to other jobs.
func ValidateState(data []byte) error {
	var state struct {
		Version          int                        `json:"version"`
		TerraformVersion string                     `json:"terraform_version"`
		Serial           *int64                     `json:"serial"`
		Lineage          string                     `json:"lineage"`
		Outputs          map[string]json.RawMessage `json:"outputs"`
		Resources        json.RawMessage            `json:"resources"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return xerrors.New("invalid Terraform state JSON")
	}
	if state.Version != 4 || state.TerraformVersion == "" || state.Serial == nil || *state.Serial < 0 || state.Lineage == "" || len(state.Resources) == 0 || string(state.Resources) == "null" {
		return xerrors.New("requires a complete Terraform v4 state snapshot")
	}
	var resources []struct {
		Mode      string            `json:"mode"`
		Instances []json.RawMessage `json:"instances"`
	}
	if err := json.Unmarshal(state.Resources, &resources); err != nil {
		return xerrors.New("invalid Terraform state resources")
	}
	for _, resource := range resources {
		if resource.Mode != "managed" && resource.Mode != "data" {
			return xerrors.New("state contains an invalid resource mode")
		}
	}
	return nil
}

// ValidateDestroyState requires a complete state with no remaining managed
// instances, including deposed instances.
func ValidateDestroyState(data []byte) error {
	return ValidateNetworkPolicyDestroyState(data)
}
