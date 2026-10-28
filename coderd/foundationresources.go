package coderd

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/coder/coder/v2/codersdk"
)

// foundationResources reads only inventory metadata from raw Terraform v4 state.
// Unreadable checkpoints must not prevent management or reconciliation of a
// Foundation. The boolean distinguishes unavailable inventory from known empty.
func foundationResources(raw []byte) ([]codersdk.FoundationResource, bool) {
	resources := []codersdk.FoundationResource{}
	if len(raw) == 0 {
		return resources, true
	}
	var state struct {
		Version   int `json:"version"`
		Resources []struct {
			Module    string      `json:"module"`
			Mode      string      `json:"mode"`
			Type      string      `json:"type"`
			Name      string      `json:"name"`
			Instances []*struct{} `json:"instances"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &state); err != nil || state.Version != 4 {
		return resources, false
	}
	for _, resource := range state.Resources {
		if resource.Mode != "managed" {
			continue
		}
		if resource.Type == "" || resource.Name == "" {
			return []codersdk.FoundationResource{}, false
		}
		for _, instance := range resource.Instances {
			if instance == nil {
				return []codersdk.FoundationResource{}, false
			}
		}
		if len(resource.Instances) == 0 {
			continue
		}
		address := resource.Type + "." + resource.Name
		if resource.Module != "" {
			address = resource.Module + "." + address
		}
		resources = append(resources, codersdk.FoundationResource{
			Address: address, Type: resource.Type, Name: resource.Name,
			Instances: len(resource.Instances),
		})
	}
	slices.SortFunc(resources, func(a, b codersdk.FoundationResource) int {
		return cmp.Compare(a.Address, b.Address)
	})
	return resources, true
}
