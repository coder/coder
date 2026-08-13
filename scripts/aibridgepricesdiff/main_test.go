package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompare(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		old         []priceRow
		new         []priceRow
		wantAdded   []string
		wantRemoved []string
		wantChanged []change
	}{
		{
			name: "identical",
			old:  []priceRow{row("anthropic", "claude", 1, 2)},
			new:  []priceRow{row("anthropic", "claude", 1, 2)},
		},
		{
			name:      "added",
			old:       []priceRow{row("anthropic", "claude", 1, 2)},
			new:       []priceRow{row("anthropic", "claude", 1, 2), row("openai", "gpt", 3, 4)},
			wantAdded: []string{"openai/gpt"},
		},
		{
			name:        "removed",
			old:         []priceRow{row("anthropic", "claude", 1, 2), row("openai", "gpt", 3, 4)},
			new:         []priceRow{row("anthropic", "claude", 1, 2)},
			wantRemoved: []string{"openai/gpt"},
		},
		{
			name: "changed",
			old:  []priceRow{row("anthropic", "claude", 1, 2)},
			new:  []priceRow{row("anthropic", "claude", 1, 5)},
			wantChanged: []change{
				{provider: "anthropic", model: "claude", field: "output", old: int64Ptr(2), new: int64Ptr(5)},
			},
		},
		{
			// A model whose price becomes null is a change, not a removal.
			name: "price unset",
			old:  []priceRow{row("anthropic", "claude", 1, 2)},
			new:  []priceRow{{Provider: "anthropic", Model: "claude", InputPrice: int64Ptr(1)}},
			wantChanged: []change{
				{provider: "anthropic", model: "claude", field: "output", old: int64Ptr(2), new: nil},
			},
		},
		{
			// Same model identifier under two providers must not collide.
			name: "same model different providers",
			old:  []priceRow{row("anthropic", "shared", 1, 2), row("openai", "shared", 1, 2)},
			new:  []priceRow{row("anthropic", "shared", 1, 2), row("openai", "shared", 9, 2)},
			wantChanged: []change{
				{provider: "openai", model: "shared", field: "input", old: int64Ptr(1), new: int64Ptr(9)},
			},
		},
		{
			name:        "added removed and changed together",
			old:         []priceRow{row("anthropic", "old-model", 1, 2), row("openai", "gpt", 3, 4)},
			new:         []priceRow{row("anthropic", "new-model", 5, 6), row("openai", "gpt", 3, 7)},
			wantAdded:   []string{"anthropic/new-model"},
			wantRemoved: []string{"anthropic/old-model"},
			wantChanged: []change{
				{provider: "openai", model: "gpt", field: "output", old: int64Ptr(4), new: int64Ptr(7)},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := compare(tc.old, tc.new)
			require.Equal(t, tc.wantAdded, names(got.added))
			require.Equal(t, tc.wantRemoved, names(got.removed))
			require.Equal(t, tc.wantChanged, got.changed)
		})
	}
}

func TestCompareSortsDeterministically(t *testing.T) {
	t.Parallel()

	old := []priceRow{}
	updated := []priceRow{
		row("openai", "b", 1, 1),
		row("anthropic", "z", 1, 1),
		row("anthropic", "a", 1, 1),
	}

	got := compare(old, updated)
	require.Equal(t, []string{"anthropic/a", "anthropic/z", "openai/b"}, names(got.added))
}

func TestRender(t *testing.T) {
	t.Parallel()

	t.Run("no changes", func(t *testing.T) {
		t.Parallel()

		out := render(compare(
			[]priceRow{row("anthropic", "claude", 1, 2)},
			[]priceRow{row("anthropic", "claude", 1, 2)},
		))
		require.Contains(t, out, "No price changes")
		require.NotContains(t, out, "### Added")
	})

	t.Run("full summary", func(t *testing.T) {
		t.Parallel()

		out := render(compare(
			[]priceRow{row("anthropic", "gone", 1_000_000, 2_000_000), row("openai", "gpt", 1_000_000, 2_000_000)},
			[]priceRow{row("anthropic", "fresh", 3_000_000, 4_000_000), row("openai", "gpt", 2_000_000, 3_000_000)},
		))

		require.Contains(t, out, "1 model added, 1 model removed, 1 model changed.")
		require.Contains(t, out, "### Added\n\n- anthropic/fresh\n")
		require.Contains(t, out, "### Removed\n\n- anthropic/gone\n")
		// A model that repriced across two fields is listed once.
		require.Contains(t, out, "### Changed\n\n- openai/gpt\n")
		require.NotContains(t, out, "| Provider |")
	})
}

func TestChangedModelNames(t *testing.T) {
	t.Parallel()

	changes := []change{
		{provider: "openai", model: "gpt", field: "input"},
		{provider: "openai", model: "gpt", field: "output"},
		{provider: "anthropic", model: "claude", field: "input"},
	}
	require.Equal(t, []string{"openai/gpt", "anthropic/claude"}, changedModelNames(changes))
}

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("missing flags", func(t *testing.T) {
		t.Parallel()

		var out strings.Builder
		require.Error(t, run("", "", &out))
	})

	t.Run("reads files", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		oldPath := filepath.Join(dir, "old.json")
		newPath := filepath.Join(dir, "new.json")
		writeFile(t, oldPath, `[{"provider":"openai","model":"gpt","input_price":1000000,"output_price":2000000,"cache_read_price":null,"cache_write_price":null}]`)
		writeFile(t, newPath, `[{"provider":"openai","model":"gpt","input_price":1500000,"output_price":2000000,"cache_read_price":null,"cache_write_price":null}]`)

		var out strings.Builder
		require.NoError(t, run(oldPath, newPath, &out))
		require.Contains(t, out.String(), "### Changed\n\n- openai/gpt\n")
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		writeFile(t, path, "{")

		var out strings.Builder
		require.Error(t, run(path, path, &out))
	})
}

func row(provider, model string, input, output int64) priceRow {
	return priceRow{
		Provider:    provider,
		Model:       model,
		InputPrice:  int64Ptr(input),
		OutputPrice: int64Ptr(output),
	}
}

func names(rows []priceRow) []string {
	if len(rows) == 0 {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Provider+"/"+r.Model)
	}
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func int64Ptr(v int64) *int64 { return &v }
