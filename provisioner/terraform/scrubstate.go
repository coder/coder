package terraform

import (
	"encoding/json"

	"golang.org/x/xerrors"
)

// scrubbedDataSources lists data source types whose attributes embed
// credentials: coder_workspace_owner holds the owner's session token, git SSH
// private key and OIDC access token, and coder_external_auth holds provider
// access tokens (SEC-1491). Data sources are re-resolved on every plan and
// apply, so removing their instances from the persisted state is safe.
// Managed resources (coder_agent, coder_env) cannot be removed without
// causing drift; they are protected by at-rest encryption instead.
var scrubbedDataSources = map[string]struct{}{
	"coder_workspace_owner": {},
	"coder_external_auth":   {},
}

// scrubState removes credential-bearing data source instances from a
// Terraform state file before it is persisted. Everything else, including
// the state's version, serial and lineage, is preserved.
func scrubState(stateContent []byte) ([]byte, error) {
	if len(stateContent) == 0 {
		return stateContent, nil
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(stateContent, &state); err != nil {
		return nil, xerrors.Errorf("unmarshal state: %w", err)
	}
	rawResources, ok := state["resources"]
	if !ok {
		return stateContent, nil
	}
	// State format v4 keeps all resources, including the ones of child
	// modules, in this single flat array.
	var resources []map[string]json.RawMessage
	if err := json.Unmarshal(rawResources, &resources); err != nil {
		return nil, xerrors.Errorf("unmarshal state resources: %w", err)
	}
	kept := make([]map[string]json.RawMessage, 0, len(resources))
	for _, resource := range resources {
		// Both fields are plain strings in every supported state version;
		// unmarshal errors leave them empty, which never matches a scrubbed
		// type, so the resource is conservatively kept.
		var mode, resourceType string
		if err := json.Unmarshal(resource["mode"], &mode); err != nil {
			mode = ""
		}
		if err := json.Unmarshal(resource["type"], &resourceType); err != nil {
			resourceType = ""
		}
		if mode == "data" {
			if _, scrub := scrubbedDataSources[resourceType]; scrub {
				continue
			}
		}
		kept = append(kept, resource)
	}
	if len(kept) == len(resources) {
		return stateContent, nil
	}
	encodedResources, err := json.Marshal(kept)
	if err != nil {
		return nil, xerrors.Errorf("marshal scrubbed resources: %w", err)
	}
	state["resources"] = encodedResources
	scrubbed, err := json.Marshal(state)
	if err != nil {
		return nil, xerrors.Errorf("marshal scrubbed state: %w", err)
	}
	return scrubbed, nil
}
