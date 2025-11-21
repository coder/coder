package rbac_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
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

	// Typed wildcard entries persist even without explicit IDs
	out, err = rbac.ParseAllowList([]string{"template:*"}, 128)
	require.NoError(t, err)
	require.Equal(t, []rbac.AllowListElement{{Type: "template", ID: "*"}}, out)

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

func TestIntersectAllowLists(t *testing.T) {
	t.Parallel()

	id := uuid.NewString()
	id2 := uuid.NewString()

	t.Run("scope_all_db_specific", func(t *testing.T) {
		t.Parallel()
		out := rbac.IntersectAllowLists(
			[]rbac.AllowListElement{rbac.AllowListAll()},
			[]rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id}},
		)
		require.Equal(t, []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id}}, out)
	})

	t.Run("db_all_keeps_scope", func(t *testing.T) {
		t.Parallel()
		scopeList := []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: policy.WildcardSymbol}}
		out := rbac.IntersectAllowLists(scopeList, []rbac.AllowListElement{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}})
		require.Equal(t, scopeList, out)
	})

	t.Run("typed_wildcard_intersection", func(t *testing.T) {
		t.Parallel()
		scopeList := []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: policy.WildcardSymbol}}
		out := rbac.IntersectAllowLists(scopeList, []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id}})
		require.Equal(t, []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id}}, out)
	})

	t.Run("db_wildcard_type_specific", func(t *testing.T) {
		t.Parallel()
		scopeList := []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id}}
		out := rbac.IntersectAllowLists(scopeList, []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: policy.WildcardSymbol}})
		require.Equal(t, []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id}}, out)
	})

	t.Run("disjoint_types", func(t *testing.T) {
		t.Parallel()
		scopeList := []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id}}
		out := rbac.IntersectAllowLists(scopeList, []rbac.AllowListElement{{Type: rbac.ResourceTemplate.Type, ID: id}})
		require.Empty(t, out)
	})

	t.Run("different_ids", func(t *testing.T) {
		t.Parallel()
		scopeList := []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: uuid.NewString()}}
		out := rbac.IntersectAllowLists(scopeList, []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id}})
		require.Empty(t, out)
	})

	t.Run("multi_entry_overlap", func(t *testing.T) {
		t.Parallel()
		templateSpecific := uuid.NewString()
		scopeList := []rbac.AllowListElement{
			{Type: rbac.ResourceWorkspace.Type, ID: id},
			{Type: rbac.ResourceWorkspace.Type, ID: id2},
			{Type: rbac.ResourceTemplate.Type, ID: policy.WildcardSymbol},
		}
		out := rbac.IntersectAllowLists(scopeList, []rbac.AllowListElement{
			{Type: rbac.ResourceWorkspace.Type, ID: id2},
			{Type: rbac.ResourceTemplate.Type, ID: templateSpecific},
			{Type: rbac.ResourceTemplate.Type, ID: policy.WildcardSymbol},
		})
		require.Equal(t, []rbac.AllowListElement{
			{Type: rbac.ResourceTemplate.Type, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceWorkspace.Type, ID: id2},
		}, out)
	})

	t.Run("multi_entry_db_wildcards", func(t *testing.T) {
		t.Parallel()
		templateID := uuid.NewString()
		dbList := []rbac.AllowListElement{
			{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol},
			{Type: rbac.ResourceWorkspace.Type, ID: id},
			{Type: rbac.ResourceTemplate.Type, ID: policy.WildcardSymbol},
		}
		out := rbac.IntersectAllowLists([]rbac.AllowListElement{
			{Type: rbac.ResourceWorkspace.Type, ID: id},
			{Type: rbac.ResourceTemplate.Type, ID: templateID},
		}, dbList)
		require.Equal(t, []rbac.AllowListElement{
			{Type: rbac.ResourceWorkspace.Type, ID: id},
			{Type: rbac.ResourceTemplate.Type, ID: templateID},
		}, out)
	})
}

func TestUnionAllowLists(t *testing.T) {
	t.Parallel()

	id1 := uuid.NewString()
	id2 := uuid.NewString()

	t.Run("wildcard_short_circuit", func(t *testing.T) {
		t.Parallel()
		out, err := rbac.UnionAllowLists(
			[]rbac.AllowListElement{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}},
			[]rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id1}},
		)
		require.NoError(t, err)
		require.Equal(t, []rbac.AllowListElement{rbac.AllowListAll()}, out)
	})

	t.Run("merge_unique_entries", func(t *testing.T) {
		t.Parallel()
		out, err := rbac.UnionAllowLists(
			[]rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id1}},
			[]rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id2}},
		)
		require.NoError(t, err)
		require.Len(t, out, 2)
		require.ElementsMatch(t, []rbac.AllowListElement{
			{Type: rbac.ResourceWorkspace.Type, ID: id1},
			{Type: rbac.ResourceWorkspace.Type, ID: id2},
		}, out)
	})

	t.Run("typed_wildcard_collapse", func(t *testing.T) {
		t.Parallel()
		out, err := rbac.UnionAllowLists(
			[]rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: policy.WildcardSymbol}},
			[]rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id1}},
		)
		require.NoError(t, err)
		require.Equal(t, []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: policy.WildcardSymbol}}, out)
	})

	t.Run("deduplicate_across_inputs", func(t *testing.T) {
		t.Parallel()
		out, err := rbac.UnionAllowLists(
			[]rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id1}},
			[]rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id1}},
		)
		require.NoError(t, err)
		require.Equal(t, []rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id1}}, out)
	})

	t.Run("combine_multiple_types", func(t *testing.T) {
		t.Parallel()
		out, err := rbac.UnionAllowLists(
			[]rbac.AllowListElement{{Type: rbac.ResourceWorkspace.Type, ID: id1}},
			[]rbac.AllowListElement{{Type: rbac.ResourceTemplate.Type, ID: id2}},
		)
		require.NoError(t, err)
		require.ElementsMatch(t, []rbac.AllowListElement{
			{Type: rbac.ResourceTemplate.Type, ID: id2},
			{Type: rbac.ResourceWorkspace.Type, ID: id1},
		}, out)
	})

	t.Run("empty_returns_empty", func(t *testing.T) {
		t.Parallel()
		out, err := rbac.UnionAllowLists(nil, []rbac.AllowListElement{})
		require.NoError(t, err)
		require.Empty(t, out)
	})
}
