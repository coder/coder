package database

import (
	"cmp"
	"slices"
)

// ChatProvidersByFamilyPrecedence selects the effective provider config for
// each family. The oldest config wins so runtime behavior matches the single-
// row lookup used elsewhere in the API.
func ChatProvidersByFamilyPrecedence(providers []ChatProvider) []ChatProvider {
	providers = slices.Clone(providers)
	slices.SortStableFunc(providers, func(a, b ChatProvider) int {
		if byProvider := cmp.Compare(a.Provider, b.Provider); byProvider != 0 {
			return byProvider
		}
		if byCreatedAt := a.CreatedAt.Compare(b.CreatedAt); byCreatedAt != 0 {
			return byCreatedAt
		}
		return cmp.Compare(a.ID.String(), b.ID.String())
	})
	selected := providers[:0]
	for _, provider := range providers {
		if len(selected) > 0 && selected[len(selected)-1].Provider == provider.Provider {
			continue
		}
		selected = append(selected, provider)
	}
	return selected
}
