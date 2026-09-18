package oauth2provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveRedirectURIs(t *testing.T) {
	t.Parallel()

	const (
		a = "https://a.example.com/callback"
		b = "https://b.example.com/callback"
		c = "https://c.example.com/callback"
	)

	tests := []struct {
		name         string
		callbackURL  string
		redirectURIs []string
		stored       []string
		want         []string
	}{
		{
			name:         "ListOnly",
			redirectURIs: []string{a, b, a},
			want:         []string{a, b},
		},
		{
			name:         "ListReplacesStored",
			redirectURIs: []string{c},
			stored:       []string{a, b},
			want:         []string{c},
		},
		{
			name:         "ListWithNewCallback",
			callbackURL:  c,
			redirectURIs: []string{a, b},
			want:         []string{c, a, b},
		},
		{
			name:         "ListWithCallbackFromList",
			callbackURL:  b,
			redirectURIs: []string{a, b},
			want:         []string{b, a},
		},
		{
			name:        "CallbackOnlyCreate",
			callbackURL: c,
			want:        []string{c},
		},
		{
			name:        "CallbackOnlyReplacesPrimary",
			callbackURL: c,
			stored:      []string{a, b},
			want:        []string{c, b},
		},
		{
			name:        "CallbackOnlyPromotesAlternate",
			callbackURL: b,
			stored:      []string{a, b},
			want:        []string{b},
		},
		{
			name:        "CallbackOnlyUnchanged",
			callbackURL: a,
			stored:      []string{a, b},
			want:        []string{a, b},
		},
		{
			name:   "NeitherKeepsStored",
			stored: []string{a, b},
			want:   []string{a, b},
		},
		{
			name: "NeitherOnCreate",
			want: []string{},
		},
		{
			name:         "ExplicitEmptyListRejected",
			redirectURIs: []string{},
			stored:       []string{a, b},
			want:         []string{},
		},
		{
			name:         "ExplicitEmptyListWithCallbackRejected",
			callbackURL:  c,
			redirectURIs: []string{},
			stored:       []string{a, b},
			want:         []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := resolveRedirectURIs(tc.callbackURL, tc.redirectURIs, tc.stored)
			require.Len(t, got, len(tc.want))
			if len(tc.want) > 0 {
				require.Equal(t, tc.want, got)
			}
		})
	}
}
