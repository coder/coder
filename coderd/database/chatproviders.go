package database

import (
	"cmp"
	"slices"
)

// SortChatProvidersByFamilyPrecedence orders provider configs by family and
// then by reverse creation order so overwrite-based key merges leave the
// oldest config as the effective winner for each family.
func SortChatProvidersByFamilyPrecedence(providers []ChatProvider) {
	slices.SortStableFunc(providers, func(a, b ChatProvider) int {
		if byProvider := cmp.Compare(a.Provider, b.Provider); byProvider != 0 {
			return byProvider
		}
		if byCreatedAt := b.CreatedAt.Compare(a.CreatedAt); byCreatedAt != 0 {
			return byCreatedAt
		}
		return cmp.Compare(b.ID.String(), a.ID.String())
	})
}
