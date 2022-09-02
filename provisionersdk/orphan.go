package provisionersdk

import (
	"encoding/json"

	"golang.org/x/xerrors"
)

// OrphanState removes all the resources from the provided
// state. When the new state is used, the provisioner will operate as if none
// of the resources in the original state exist.
func OrphanState(state []byte) ([]byte, error) {
	if len(state) == 0 {
		// Presume that state is already orphaned, or we're using
		// a no-op provisioner.
		return state, nil
	}

	stateMap := make(map[string]interface{})
	err := json.Unmarshal(state, &stateMap)
	if err != nil {
		return nil, err
	}

	_, ok := stateMap["resources"]
	if !ok {
		return nil, xerrors.Errorf("no resources detected, is this terraform state?")
	}

	// Terraform wants a resources array.
	stateMap["resources"] = []struct{}{}

	return json.Marshal(stateMap)
}
