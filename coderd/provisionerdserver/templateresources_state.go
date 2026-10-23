package provisionerdserver

import (
	"encoding/json"

	"golang.org/x/xerrors"
)

const maxTemplateResourceOutputsSize = 64 * 1024

// templateResourceOutputs reads root outputs from raw Terraform state, never
// from a plan or terraform show JSON. RawMessage preserves numeric precision.
func templateResourceOutputs(data []byte, action string) (json.RawMessage, error) {
	var state struct {
		Version          int    `json:"version"`
		TerraformVersion string `json:"terraform_version"`
		Serial           *int64 `json:"serial"`
		Lineage          string `json:"lineage"`
		Outputs          map[string]struct {
			Sensitive bool            `json:"sensitive"`
			Value     json.RawMessage `json:"value"`
		} `json:"outputs"`
		Resources json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, xerrors.Errorf("decode template resource state: %w", err)
	}
	if state.Version != 4 || state.TerraformVersion == "" || state.Serial == nil || *state.Serial < 0 || state.Lineage == "" || len(state.Resources) == 0 || string(state.Resources) == "null" {
		return nil, xerrors.New("template resources require a complete Terraform v4 state snapshot")
	}
	var resources []struct {
		Mode      string          `json:"mode"`
		Instances json.RawMessage `json:"instances"`
	}
	if err := json.Unmarshal(state.Resources, &resources); err != nil {
		return nil, xerrors.Errorf("decode template resource instances: %w", err)
	}
	for _, resource := range resources {
		var instances []json.RawMessage
		if len(resource.Instances) == 0 || string(resource.Instances) == "null" {
			return nil, xerrors.New("template resource state is missing its instance list")
		}
		if err := json.Unmarshal(resource.Instances, &instances); err != nil {
			return nil, xerrors.Errorf("decode template resource instance list: %w", err)
		}
		switch resource.Mode {
		case "managed":
			if action == "destroy" && len(instances) != 0 {
				return nil, xerrors.New("destroy state still contains managed instances")
			}
		case "data":
		default:
			return nil, xerrors.New("template resource state contains an invalid resource mode")
		}
	}
	outputs := map[string]json.RawMessage{}
	for name, output := range state.Outputs {
		// Terraform v4 state omits sensitive when its value is false.
		if output.Sensitive {
			continue
		}
		if len(output.Value) == 0 {
			return nil, xerrors.New("template resource output is missing its value")
		}
		outputs[name] = output.Value
	}
	if action == "destroy" {
		outputs = map[string]json.RawMessage{}
	}
	return marshalTemplateResourceOutputs(outputs)
}

func marshalTemplateResourceOutputs(outputs map[string]json.RawMessage) (json.RawMessage, error) {
	if outputs == nil {
		outputs = map[string]json.RawMessage{}
	}
	encoded, err := json.Marshal(outputs)
	if err != nil {
		return nil, xerrors.Errorf("encode template resource outputs: %w", err)
	}
	if len(encoded) > maxTemplateResourceOutputsSize {
		return nil, xerrors.New("template resource outputs exceed the 64 KiB limit")
	}
	return encoded, nil
}

// templateResourceOutputsMetadata bounds the actual environment payload after
// compacting the JSONB representation stored by Postgres.
func templateResourceOutputsMetadata(data json.RawMessage) (string, error) {
	var outputs map[string]json.RawMessage
	if len(data) != 0 {
		if err := json.Unmarshal(data, &outputs); err != nil {
			return "", xerrors.Errorf("decode template resource output snapshot: %w", err)
		}
	}
	encoded, err := marshalTemplateResourceOutputs(outputs)
	return string(encoded), err
}
