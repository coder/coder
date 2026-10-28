package coderd

import (
	"bytes"
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

// foundationPublicOutputs deliberately separates public values from private
// provisioning inputs. Do not decode the entire snapshot into an API response.
func foundationPublicOutputs(data json.RawMessage, result *codersdk.Foundation) error {
	snapshot, err := provisionersdk.DecodeFoundationOutputSnapshot(data)
	if err != nil {
		return err
	}
	result.Outputs = map[string]interface{}{}
	result.OutputMetadata = map[string]codersdk.FoundationOutputMetadata{}
	result.OutputSnapshotID = snapshot.SnapshotID
	for name, value := range snapshot.Values {
		metadata := snapshot.Metadata[name]
		result.OutputMetadata[name] = codersdk.FoundationOutputMetadata{Type: metadata.Type, Sensitive: metadata.Sensitive}
		if metadata.Sensitive {
			continue
		}
		var decoded interface{}
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err != nil {
			return xerrors.New("invalid Foundation output value")
		}
		result.Outputs[name] = decoded
	}
	return nil
}
