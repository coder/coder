package provisionersdk

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/provisionersdk/proto"
)

// ResourceAddresses returns an index-matching slice of unique addresses
// to access resources.
func ResourceAddresses(resources []*proto.Resource) ([]string, error) {
	resourcesByHost := map[string]*proto.Resource{}
	for _, resource := range resources {
		otherByName, exists := resourcesByHost[resource.Name]
		if !exists {
			resourcesByHost[resource.Name] = resource
			continue
		}
		// If we have conflicting names, to reduce confusion we prepend the types.
		delete(resourcesByHost, otherByName.Name)
		otherAddress := fmt.Sprintf("%s.%s", otherByName.Type, otherByName.Name)
		resourcesByHost[otherAddress] = otherByName
		address := fmt.Sprintf("%s.%s", resource.Type, resource.Name)
		_, exists = resourcesByHost[address]
		if !exists {
			resourcesByHost[address] = resource
			continue
		}
		return nil, xerrors.Errorf("found resource with conflicting address %q", otherAddress)
	}

	addresses := make([]string, 0, len(resources))
	for _, resource := range resources {
		found := false
		for host, other := range resourcesByHost {
			if resource != other {
				continue
			}
			found = true
			addresses = append(addresses, host)
		}
		if !found {
			panic(fmt.Sprintf("dev error: resource %s.%s wasn't given an address", resource.Type, resource.Name))
		}
	}
	return addresses, nil
}
