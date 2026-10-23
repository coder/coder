package provisionersdk_test

import (
	"maps"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk"
)

func TestNetworkPolicyProvisionerTags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, version string
		tags          map[string]string
		wantDestroy   bool
	}{
		{name: "Ordinary", version: "1.22"},
		{name: "PlanApplyOnly", version: "1.21", tags: map[string]string{provisionersdk.TagNetworkPolicy: "1"}},
		{name: "Destroy", version: "1.22", tags: map[string]string{provisionersdk.TagNetworkPolicy: "1"}, wantDestroy: true},
		{name: "NewerMinor", version: "1.23", tags: map[string]string{provisionersdk.TagNetworkPolicy: "1"}, wantDestroy: true},
		{name: "OldWorkerConfiguredTag", version: "1.21", tags: map[string]string{provisionersdk.TagNetworkPolicy: "1", provisionersdk.TagNetworkPolicyDestroy: "1"}},
		{name: "NotOptedIn", version: "1.22", tags: map[string]string{provisionersdk.TagNetworkPolicy: "0", provisionersdk.TagNetworkPolicyDestroy: "1"}},
		{name: "InvalidVersion", version: "invalid", tags: map[string]string{provisionersdk.TagNetworkPolicy: "1", provisionersdk.TagNetworkPolicyDestroy: "1"}},
		{name: "UnknownMajor", version: "2.22", tags: map[string]string{provisionersdk.TagNetworkPolicy: "1", provisionersdk.TagNetworkPolicyDestroy: "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			original := maps.Clone(tc.tags)
			got := provisionersdk.NetworkPolicyProvisionerTags(tc.tags, tc.version)
			want := maps.Clone(tc.tags)
			delete(want, provisionersdk.TagNetworkPolicyDestroy)
			if tc.wantDestroy {
				want[provisionersdk.TagNetworkPolicyDestroy] = "1"
			}
			require.Equal(t, want, got)
			require.Equal(t, original, tc.tags, "registration must not mutate configured tags")
		})
	}
}

func TestTemplateResourcesProvisionerTags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, version, capability string
		want                      bool
	}{
		{name: "OlderWorker", version: "1.22", capability: "1"},
		{name: "OptedIn", version: "1.23", capability: "1", want: true},
		{name: "NewerWorker", version: "1.24", capability: "1", want: true},
		{name: "NotOptedIn", version: "1.23"},
		{name: "InvalidCapability", version: "1.23", capability: "2"},
		{name: "InvalidVersion", version: "invalid", capability: "1"},
		{name: "UnknownMajor", version: "2.23", capability: "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tags := map[string]string{"other": "value", provisionersdk.TagNetworkPolicyDestroy: "1"}
			if tc.capability != "" {
				tags[provisionersdk.TagTemplateResources] = tc.capability
			}
			original := maps.Clone(tags)
			got := provisionersdk.TemplateResourcesProvisionerTags(tags, tc.version)
			require.Equal(t, original, tags)
			require.Equal(t, "value", got["other"])
			require.Equal(t, "1", got[provisionersdk.TagNetworkPolicyDestroy], "old policy capability is independent")
			if tc.want {
				require.Equal(t, "1", got[provisionersdk.TagTemplateResources])
			} else {
				require.NotContains(t, got, provisionersdk.TagTemplateResources)
			}
		})
	}
	require.Nil(t, provisionersdk.TemplateResourcesProvisionerTags(nil, "1.23"))
}

func TestMutateTags(t *testing.T) {
	t.Parallel()

	testUserID := uuid.New()

	for _, tt := range []struct {
		name   string
		userID uuid.UUID
		tags   []map[string]string
		want   map[string]string
	}{
		{
			name:   "nil tags",
			userID: uuid.Nil,
			tags:   []map[string]string{nil},
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name:   "empty tags",
			userID: uuid.Nil,
			tags:   []map[string]string{{}},
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name: "user scope",
			tags: []map[string]string{
				{provisionersdk.TagScope: provisionersdk.ScopeUser},
			},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeUser,
				provisionersdk.TagOwner: testUserID.String(),
			},
		},
		{
			name: "organization scope",
			tags: []map[string]string{
				{provisionersdk.TagScope: provisionersdk.ScopeOrganization},
			},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name: "organization scope with owner",
			tags: []map[string]string{
				{
					provisionersdk.TagScope: provisionersdk.ScopeOrganization,
					provisionersdk.TagOwner: testUserID.String(),
				},
			},
			userID: uuid.Nil,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name: "owner tag with no other context",
			tags: []map[string]string{
				{
					provisionersdk.TagOwner: testUserID.String(),
				},
			},
			userID: uuid.Nil,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name: "invalid scope",
			tags: []map[string]string{
				{provisionersdk.TagScope: "360noscope"},
			},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name: "merge two empty maps",
			tags: []map[string]string{
				{},
				{},
			},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
		{
			name: "merge empty map with non-empty map",
			tags: []map[string]string{
				{},
				{"foo": "bar"},
			},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
				"foo":                   "bar",
			},
		},
		{
			name: "merge non-empty map with empty map",
			tags: []map[string]string{
				{"foo": "bar"},
				{},
			},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
				"foo":                   "bar",
			},
		},
		{
			name: "merge map with same map",
			tags: []map[string]string{
				{"foo": "bar"},
				{"foo": "bar"},
			},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
				"foo":                   "bar",
			},
		},
		{
			name: "merge map with override",
			tags: []map[string]string{
				{"foo": "bar"},
				{"foo": "baz"},
			},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
				"foo":                   "baz",
			},
		},
		{
			name: "do not override empty in second map",
			tags: []map[string]string{
				{"foo": "bar"},
				{"foo": ""},
			},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
				"foo":                   "bar",
			},
		},
		{
			name:   "merge nil map with nil map",
			tags:   []map[string]string{nil, nil},
			userID: testUserID,
			want: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
				provisionersdk.TagOwner: "",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := provisionersdk.MutateTags(tt.userID, tt.tags...)
			require.Equal(t, tt.want, got)
		})
	}
}
