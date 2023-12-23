package provisionersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/coder/coder/v2/provisionersdk"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMutateTags(t *testing.T) {
	t.Parallel()

	testUserID := uuid.New()

	for _, tt := range []struct {
		name   string
		userID uuid.UUID
		tags   map[string]string
		want   map[string]string
	}{
		{
			name:   "nil tags",
			userID: uuid.Nil,
			tags:   nil,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name:   "empty tags",
			userID: uuid.Nil,
			tags:   map[string]string{},
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name:   "user scope",
			tags:   map[string]string{provisionersdk.TagScope: provisionersdk.ScopeUser},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeUser,
				provisionersdk.TagOwner: testUserID.String(),
			},
		},
		{
			name:   "organization scope",
			tags:   map[string]string{provisionersdk.TagScope: provisionersdk.ScopeOrganization},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name: "organization scope with owner",
			tags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: testUserID.String(),
			},
			userID: uuid.Nil,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name: "owner tag with no other context",
			tags: map[string]string{
				provisionersdk.TagOwner: testUserID.String(),
			},
			userID: uuid.Nil,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name:   "invalid scope",
			tags:   map[string]string{provisionersdk.TagScope: "360noscope"},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// make a copy of the map because the function under test
			// mutates the map
			bytes, err := json.Marshal(tt.tags)
			require.NoError(t, err)
			var tags map[string]string
			err = json.Unmarshal(bytes, &tags)
			require.NoError(t, err)
			got := provisionersdk.MutateTags(tt.userID, tags)
			require.Equal(t, tt.want, got)
		})
	}
}
