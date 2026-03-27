package database

import (
	"cmp"
	"slices"
)

// SortChatProvidersByFamilyPrecedence orders provider configs by family and
// then by creation time so callers resolve duplicate enabled configs
// deterministically.
func SortChatProvidersByFamilyPrecedence(providers []ChatProvider) {
	slices.SortStableFunc(providers, func(a, b ChatProvider) int {
		if byProvider := cmp.Compare(a.Provider, b.Provider); byProvider != 0 {
			return byProvider
		}
		if byCreatedAt := a.CreatedAt.Compare(b.CreatedAt); byCreatedAt != 0 {
			return byCreatedAt
		}
		return cmp.Compare(a.ID.String(), b.ID.String())
	})
}
