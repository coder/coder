package provisionersdk

import (
	"encoding/json"

	"golang.org/x/xerrors"
)

// FoundationOutputMetadata preserves a root output's Terraform type and sensitivity.
type FoundationOutputMetadata struct {
	Type      json.RawMessage `json:"type,omitempty"`
	Sensitive bool            `json:"sensitive"`
}

// FoundationOutputSnapshot is the private, immutable output contract of an apply.
// Values may contain secrets and must not be returned by management APIs or logged.
type FoundationOutputSnapshot struct {
	Values     map[string]json.RawMessage          `json:"values"`
	Metadata   map[string]FoundationOutputMetadata `json:"metadata"`
	SnapshotID string                              `json:"snapshot_id,omitempty"`
}

// DecodeFoundationOutputSnapshot decodes a stored output contract. Empty snapshots
// and value-only fixtures have no sensitivity or type metadata.
func DecodeFoundationOutputSnapshot(data json.RawMessage) (FoundationOutputSnapshot, error) {
	result := FoundationOutputSnapshot{
		Values:   map[string]json.RawMessage{},
		Metadata: map[string]FoundationOutputMetadata{},
	}
	if len(data) == 0 {
		return result, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return result, xerrors.New("invalid Foundation output snapshot")
	}
	if _, ok := fields["metadata"]; ok && fields["values"] != nil {
		if err := json.Unmarshal(data, &result); err != nil {
			return result, xerrors.New("invalid Foundation output contract")
		}
	} else if fields != nil {
		result.Values = fields
	}
	if result.Values == nil {
		result.Values = map[string]json.RawMessage{}
	}
	if result.Metadata == nil {
		result.Metadata = map[string]FoundationOutputMetadata{}
	}
	return result, nil
}
