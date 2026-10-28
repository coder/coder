package provisionerdserver

import (
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
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
			Type      json.RawMessage `json:"type"`
		} `json:"outputs"`
		Resources json.RawMessage `json:"resources"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, xerrors.Errorf("decode shared resource state: %w", err)
	}
	if state.Version != 4 || state.TerraformVersion == "" || state.Serial == nil || *state.Serial < 0 || state.Lineage == "" || len(state.Resources) == 0 || string(state.Resources) == "null" {
		return nil, xerrors.New("shared resources require a complete Terraform v4 state snapshot")
	}
	var resources []struct {
		Mode      string          `json:"mode"`
		Instances json.RawMessage `json:"instances"`
	}
	if err := json.Unmarshal(state.Resources, &resources); err != nil {
		return nil, xerrors.Errorf("decode shared resource instances: %w", err)
	}
	for _, resource := range resources {
		var instances []json.RawMessage
		if len(resource.Instances) == 0 || string(resource.Instances) == "null" {
			return nil, xerrors.New("shared resource state is missing its instance list")
		}
		if err := json.Unmarshal(resource.Instances, &instances); err != nil {
			return nil, xerrors.Errorf("decode shared resource instance list: %w", err)
		}
		switch resource.Mode {
		case "managed":
			if action == "destroy" && len(instances) != 0 {
				return nil, xerrors.New("destroy state still contains managed instances")
			}
		case "data":
		default:
			return nil, xerrors.New("shared resource state contains an invalid resource mode")
		}
	}
	snapshot := provisionersdk.FoundationOutputSnapshot{
		Values:   map[string]json.RawMessage{},
		Metadata: map[string]provisionersdk.FoundationOutputMetadata{},
	}
	for name, output := range state.Outputs {
		if len(output.Value) == 0 {
			return nil, xerrors.New("shared resource output is missing its value")
		}
		snapshot.Values[name] = output.Value
		snapshot.Metadata[name] = provisionersdk.FoundationOutputMetadata{Type: output.Type, Sensitive: output.Sensitive}
	}
	if action == "destroy" {
		return json.RawMessage(`{}`), nil
	}
	if _, err := marshalTemplateResourceOutputs(snapshot.Values); err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(snapshot.Metadata)
	if err != nil || len(metadata) > maxTemplateResourceOutputsSize {
		return nil, xerrors.New("Foundation output type metadata exceeds the 64 KiB limit")
	}
	return json.Marshal(snapshot)
}

func marshalTemplateResourceOutputs(outputs map[string]json.RawMessage) (json.RawMessage, error) {
	if outputs == nil {
		outputs = map[string]json.RawMessage{}
	}
	encoded, err := json.Marshal(outputs)
	if err != nil {
		return nil, xerrors.Errorf("encode shared resource outputs: %w", err)
	}
	if len(encoded) > maxTemplateResourceOutputsSize {
		return nil, xerrors.New("shared resource outputs exceed the 64 KiB limit")
	}
	return encoded, nil
}

// templateResourceOutputsMetadata bounds the actual environment payload after
// compacting the JSONB representation stored by Postgres.
func templateResourceOutputsMetadata(data json.RawMessage) (string, error) {
	snapshot, err := provisionersdk.DecodeFoundationOutputSnapshot(data)
	if err != nil {
		return "", err
	}
	encoded, err := marshalTemplateResourceOutputs(snapshot.Values)
	return string(encoded), err
}

func stampFoundationOutputSnapshot(data json.RawMessage, runID uuid.UUID) (json.RawMessage, error) {
	snapshot, err := provisionersdk.DecodeFoundationOutputSnapshot(data)
	if err != nil {
		return nil, err
	}
	snapshot.SnapshotID = runID.String()
	return json.Marshal(snapshot)
}

func populateFoundationOutputMetadata(data json.RawMessage, metadata *sdkproto.Metadata) error {
	snapshot, err := provisionersdk.DecodeFoundationOutputSnapshot(data)
	if err != nil {
		return err
	}
	values, err := marshalTemplateResourceOutputs(snapshot.Values)
	if err != nil {
		return err
	}
	types, err := json.Marshal(snapshot.Metadata)
	if err != nil || len(types) > maxTemplateResourceOutputsSize {
		return xerrors.New("Foundation output type metadata exceeds the 64 KiB limit")
	}
	metadata.TemplateOutputs = string(values)
	metadata.FoundationOutputMetadata = string(types)
	metadata.FoundationSnapshotId = snapshot.SnapshotID
	return nil
}
