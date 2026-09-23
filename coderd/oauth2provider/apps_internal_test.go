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
			name:         "ListWithMatchingCallback",
			callbackURL:  a,
			redirectURIs: []string{a, b},
			stored:       []string{c},
			want:         []string{a, b},
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
			want: nil,
		},
		{
			name:         "ExplicitEmptyListIgnoresStored",
			redirectURIs: []string{},
			stored:       []string{a, b},
			want:         []string{},
		},
		{
			name:         "ExplicitEmptyListIgnoresCallback",
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
			require.Equal(t, tc.want, got)
		})
	}
}

func TestValidateRedirectURIFieldsAgree(t *testing.T) {
	t.Parallel()

	const (
		a = "https://a.example.com/callback"
		b = "https://b.example.com/callback"
	)

	tests := []struct {
		name         string
		callbackURL  string
		redirectURIs []string
		wantErr      bool
	}{
		{name: "ListOnly", redirectURIs: []string{a, b}},
		{name: "CallbackOnly", callbackURL: a},
		{name: "CallbackWithEmptyList", callbackURL: a, redirectURIs: []string{}},
		{name: "CallbackMatchesFirst", callbackURL: a, redirectURIs: []string{a, b}},
		{name: "CallbackMatchesLater", callbackURL: b, redirectURIs: []string{a, b}, wantErr: true},
		{name: "CallbackNotInList", callbackURL: "https://c.example.com/callback", redirectURIs: []string{a, b}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			errs := validateRedirectURIFieldsAgree(tc.callbackURL, tc.redirectURIs)
			if !tc.wantErr {
				require.Nil(t, errs)
				return
			}
			require.Len(t, errs, 1)
			require.Equal(t, "callback_url", errs[0].Field)
		})
	}
}
