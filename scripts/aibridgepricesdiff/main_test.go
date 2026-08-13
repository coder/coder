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
			[]priceRow{row("anthropic", "fresh", 3_000_000, 4_000_000), row("openai", "gpt", 2_000_000, 2_000_000)},
		))

		require.Contains(t, out, "1 model added, 1 model removed, 1 price changed.")
		require.Contains(t, out, "| anthropic | fresh | 3 | 4 | unset | unset |")
		require.Contains(t, out, "| anthropic | gone | 1 | 2 | unset | unset |")
		require.Contains(t, out, "| openai | gpt | input | 1 | 2 | +100.0% |")
	})
}

func TestFormatPrice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   *int64
		want string
	}{
		{"missing", nil, "unset"},
		{"zero", int64Ptr(0), "0"},
		{"whole", int64Ptr(10_000_000), "10"},
		{"fractional", int64Ptr(75_000), "0.075"},
		{"sub micro unit", int64Ptr(1), "0.000001"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, formatPrice(tc.in))
		})
	}
}

func TestFormatDelta(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		prev, next *int64
		want       string
	}{
		{"increase", int64Ptr(1_000_000), int64Ptr(2_000_000), "+100.0%"},
		{"decrease", int64Ptr(2_000_000), int64Ptr(1_000_000), "-50.0%"},
		{"newly priced", nil, int64Ptr(1), "newly priced"},
		{"price removed", int64Ptr(1), nil, "price removed"},
		{"was free", int64Ptr(0), int64Ptr(1), "was free"},
		{"both missing", nil, nil, "n/a"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, formatDelta(tc.prev, tc.next))
		})
	}
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
		require.Contains(t, out.String(), "| openai | gpt | input | 1 | 1.5 | +50.0% |")
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
