package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func Test_removeMainlineBlurb(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "NoMainlineBlurb",
			body: `## Changelog

### Chores

- Add support for additional Azure Instance Identity RSA Certificates (#13028) (@kylecarbs)

Compare: [` + "`" + `v2.10.1...v2.10.2` + "`" + `](https://github.com/coder/coder/compare/v2.10.1...v2.10.2)

## Container image

- ` + "`" + `docker pull ghcr.io/coder/coder:v2.10.2` + "`" + `

## Install/upgrade

Refer to our docs to [install](https://coder.com/docs/v2/latest/install) or [upgrade](https://coder.com/docs/v2/latest/admin/upgrade) Coder, or use a release asset below.
`,
			want: `## Changelog

### Chores

- Add support for additional Azure Instance Identity RSA Certificates (#13028) (@kylecarbs)

Compare: [` + "`" + `v2.10.1...v2.10.2` + "`" + `](https://github.com/coder/coder/compare/v2.10.1...v2.10.2)

## Container image

- ` + "`" + `docker pull ghcr.io/coder/coder:v2.10.2` + "`" + `

## Install/upgrade

Refer to our docs to [install](https://coder.com/docs/v2/latest/install) or [upgrade](https://coder.com/docs/v2/latest/admin/upgrade) Coder, or use a release asset below.
`,
		},
		{
			name: "WithMainlineBlurb",
			body: `## Changelog

> [!NOTE]
> This is a mainline Coder release. We advise enterprise customers without a staging environment to install our [latest stable release](https://github.com/coder/coder/releases/latest) while we refine this version. Learn more about our [Release Schedule](https://coder.com/docs/v2/latest/install/releases).

### Chores

- Add support for additional Azure Instance Identity RSA Certificates (#13028) (@kylecarbs)

Compare: [` + "`" + `v2.10.1...v2.10.2` + "`" + `](https://github.com/coder/coder/compare/v2.10.1...v2.10.2)

## Container image

- ` + "`" + `docker pull ghcr.io/coder/coder:v2.10.2` + "`" + `

## Install/upgrade

Refer to our docs to [install](https://coder.com/docs/v2/latest/install) or [upgrade](https://coder.com/docs/v2/latest/admin/upgrade) Coder, or use a release asset below.
`,
			want: `## Changelog

### Chores

- Add support for additional Azure Instance Identity RSA Certificates (#13028) (@kylecarbs)

Compare: [` + "`" + `v2.10.1...v2.10.2` + "`" + `](https://github.com/coder/coder/compare/v2.10.1...v2.10.2)

## Container image

- ` + "`" + `docker pull ghcr.io/coder/coder:v2.10.2` + "`" + `

## Install/upgrade

Refer to our docs to [install](https://coder.com/docs/v2/latest/install) or [upgrade](https://coder.com/docs/v2/latest/admin/upgrade) Coder, or use a release asset below.
`,
		},
		{
			name: "EntireQuotedBlurbIsRemoved",
			body: `## Changelog

> [!NOTE]
> This is a mainline Coder release. We advise enterprise customers without a staging environment to install our [latest stable release](https://github.com/coder/coder/releases/latest) while we refine this version. Learn more about our [Release Schedule](https://coder.com/docs/v2/latest/install/releases).
> This is an extended note.
> This is another extended note.

### Best release yet!

Enjoy.
`,
			want: `## Changelog

### Best release yet!

Enjoy.
`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(removeMainlineBlurb(tt.body), tt.want); diff != "" {
				require.Fail(t, "removeMainlineBlurb() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_addStableSince(t *testing.T) {
	t.Parallel()

	date := time.Date(2024, time.April, 23, 0, 0, 0, 0, time.UTC)
	body := "## Changelog"

	expected := "> ## Stable (since April 23, 2024)\n\n## Changelog"
	result := addStableSince(date, body)

	if diff := cmp.Diff(expected, result); diff != "" {
		require.Fail(t, "addStableSince() mismatch (-want +got):\n%s", diff)
	}
}

func Test_release_autoversion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := filepath.Join("testdata", "autoversion")

	fs := afero.NewCopyOnWriteFs(afero.NewOsFs(), afero.NewMemMapFs())
	r := releaseCommand{
		fs: afero.NewBasePathFs(fs, dir),
	}

	err := r.autoversion(ctx, "mainline", "v2.11.1")
	require.NoError(t, err)

	err = r.autoversion(ctx, "stable", "v2.9.4")
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(dir, "docs", "*.md"))
	require.NoError(t, err)

	for _, file := range files {
		file := file
		t.Run(file, func(t *testing.T) {
			t.Parallel()

			got, err := afero.ReadFile(fs, file)
			require.NoError(t, err)

			want, err := afero.ReadFile(fs, file+".golden")
			require.NoError(t, err)

			if diff := cmp.Diff(string(got), string(want)); diff != "" {
				require.Failf(t, "mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
