package rbac_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
)

func TestParseAllowListEntry(t *testing.T) {
	t.Parallel()
	e, err := rbac.ParseAllowListEntry("*:*")
	require.NoError(t, err)
	require.Equal(t, rbac.AllowListElement{Type: "*", ID: "*"}, e)

	e, err = rbac.ParseAllowListEntry("workspace:*")
	require.NoError(t, err)
	require.Equal(t, rbac.AllowListElement{Type: "workspace", ID: "*"}, e)

	id := uuid.New().String()
	e, err = rbac.ParseAllowListEntry("template:" + id)
	require.NoError(t, err)
	require.Equal(t, rbac.AllowListElement{Type: "template", ID: id}, e)

	_, err = rbac.ParseAllowListEntry("unknown:*")
	require.Error(t, err)
	_, err = rbac.ParseAllowListEntry("workspace:bad-uuid")
	require.Error(t, err)
	_, err = rbac.ParseAllowListEntry(":")
	require.Error(t, err)
}

func TestParseAllowListNormalize(t *testing.T) {
	t.Parallel()
	id1 := uuid.New().String()
	id2 := uuid.New().String()

	// Global wildcard short-circuits
	out, err := rbac.ParseAllowList([]string{"workspace:" + id1, "*:*", "template:" + id2}, 128)
	require.NoError(t, err)
	require.Equal(t, []rbac.AllowListElement{{Type: "*", ID: "*"}}, out)

	// Typed wildcard collapses typed ids
	out, err = rbac.ParseAllowList([]string{"workspace:*", "workspace:" + id1, "workspace:" + id2}, 128)
	require.NoError(t, err)
	require.Equal(t, []rbac.AllowListElement{{Type: "workspace", ID: "*"}}, out)

	// Dedup ids and sort deterministically
	out, err = rbac.ParseAllowList([]string{"template:" + id2, "template:" + id2, "template:" + id1}, 128)
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, "template", out[0].Type)
	require.Equal(t, "template", out[1].Type)
}

func TestParseAllowListLimit(t *testing.T) {
	t.Parallel()
	inputs := make([]string, 0, 130)
	for range 130 {
		inputs = append(inputs, "workspace:"+uuid.New().String())
	}
	_, err := rbac.ParseAllowList(inputs, 128)
	require.Error(t, err)
}
