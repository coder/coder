package provisionerdserver_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/provisionerdserver"
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
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
			},
		},
		{
			name:   "empty tags",
			userID: uuid.Nil,
			tags:   map[string]string{},
			want: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
			},
		},
		{
			name:   "user scope",
			tags:   map[string]string{provisionerdserver.TagScope: provisionerdserver.ScopeUser},
			userID: testUserID,
			want: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeUser,
				provisionerdserver.TagOwner: testUserID.String(),
			},
		},
		{
			name:   "organization scope",
			tags:   map[string]string{provisionerdserver.TagScope: provisionerdserver.ScopeOrganization},
			userID: testUserID,
			want: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
			},
		},
		{
			name:   "invalid scope",
			tags:   map[string]string{provisionerdserver.TagScope: "360noscope"},
			userID: testUserID,
			want: map[string]string{
				provisionerdserver.TagScope: provisionerdserver.ScopeOrganization,
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
			got := provisionerdserver.MutateTags(tt.userID, tags)
			require.Equal(t, tt.want, got)
		})
	}
}
