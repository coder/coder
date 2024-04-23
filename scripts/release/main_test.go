package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
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
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(removeMainlineBlurb(tt.body), tt.want); diff != "" {
				t.Errorf("removeMainlineBlurb() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
