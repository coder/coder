package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixtureSwagger has two tags. Alpha owns codersdk.Shared (and its nested
// type) because it comes first in the default order of the test, even though
// Beta reaches them too. codersdk.OnlySkipped is reached only by a skipped op.
const fixtureSwagger = `{
  "tags": [{"name": "Alpha", "description": "Alpha things."}, {"name": "Beta"}],
  "paths": {
    "/a": {
      "get": {
        "tags": ["Alpha"],
        "summary": "List a",
        "parameters": [
          {"name": "limit", "in": "query", "description": "Page limit"},
          {"name": "q", "in": "query", "description": "Search query matching many fields"}
        ],
        "responses": {"200": {"schema": {"$ref": "#/definitions/codersdk.Shared"}}}
      },
      "post": {
        "tags": ["Alpha"],
        "summary": "Create a",
        "description": "Creates an a.",
        "responses": {}
      }
    },
    "/b": {
      "get": {
        "tags": ["Beta"],
        "summary": "Get b",
        "parameters": [{"name": "body", "in": "body", "schema": {"$ref": "#/definitions/codersdk.BetaOnly"}}],
        "responses": {"200": {"schema": {"$ref": "#/definitions/codersdk.Shared"}}}
      }
    },
    "/skipped": {
      "get": {
        "tags": ["Beta"],
        "summary": "Skipped",
        "x-apidocgen": {"skip": true},
        "responses": {"200": {"schema": {"$ref": "#/definitions/codersdk.OnlySkipped"}}}
      }
    }
  },
  "definitions": {
    "codersdk.Shared": {
      "type": "object",
      "properties": {
        "id": {"type": "string", "description": "The ID."},
        "name": {"type": "string"},
        "nested": {"$ref": "#/definitions/codersdk.Nested"}
      }
    },
    "codersdk.Nested": {
      "type": "object",
      "properties": {"value": {"type": "string", "description": "  "}}
    },
    "codersdk.BetaOnly": {
      "type": "object",
      "properties": {"flag": {"type": "boolean"}}
    },
    "codersdk.OnlySkipped": {
      "type": "object",
      "properties": {"secret": {"type": "string"}}
    },
    "other.Thing": {
      "type": "object",
      "properties": {"x": {"type": "string"}}
    }
  }
}`

const fixtureShared = "package codersdk\r\n\r\n" +
	"// Shared is documented.\r\n" +
	"type Shared struct {\r\n\tID string `json:\"id\"`\r\n}\r\n"

const fixtureGrouped = "package codersdk\r\n\r\n" +
	"type (\r\n" +
	"\tNested struct {\r\n\t\tValue string `json:\"value\"`\r\n\t}\r\n\r\n" +
	"\tBetaOnly struct{}\r\n" +
	")\r\n"

// fixtureTestOnly must be ignored, so OnlySkipped stays unmapped.
const fixtureTestOnly = "package codersdk\r\n\r\ntype OnlySkipped struct{}\r\n"

func writeFixtures(t *testing.T) (swaggerPath, sdkDir string) {
	t.Helper()
	dir := t.TempDir()
	sdkDir = filepath.Join(dir, "codersdk")
	require.NoError(t, os.Mkdir(sdkDir, 0o755))
	for name, content := range map[string]string{
		"shared.go":       fixtureShared,
		"grouped.go":      fixtureGrouped,
		"skipped_test.go": fixtureTestOnly,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(sdkDir, name), []byte(content), 0o600))
	}
	swaggerPath = filepath.Join(dir, "swagger.json")
	require.NoError(t, os.WriteFile(swaggerPath, []byte(fixtureSwagger), 0o600))
	return swaggerPath, sdkDir
}

func TestMapTypesToFiles_CRLF(t *testing.T) {
	t.Parallel()

	_, sdkDir := writeFixtures(t)
	got, err := mapTypesToFiles(sdkDir)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"Shared":   "shared.go",
		"Nested":   "grouped.go",
		"BetaOnly": "grouped.go",
	}, got)
}

func TestMapTypesToFiles_ParseError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.go"), []byte("not go"), 0o600))
	_, err := mapTypesToFiles(dir)
	require.ErrorContains(t, err, "bad.go")
}

func TestRun(t *testing.T) {
	t.Parallel()

	swaggerPath, sdkDir := writeFixtures(t)
	base := []string{"-swagger", swaggerPath, "-codersdk", sdkDir, "-order", "Alpha,Beta"}

	tests := []struct {
		name    string
		extra   []string
		want    []string
		notWant []string
		wantErr string
	}{
		{
			name: "SummaryOwnership",
			// Alpha owns Shared and Nested (3 of 4 fields undocumented),
			// although Beta reaches Shared too. Beta owns only BetaOnly.
			// The skipped op is counted separately.
			want: []string{
				"| Alpha | 2 | 1 | 2 | 1 | 3 of 4 | `shared.go`, `grouped.go` | 0 |",
				"| Beta | 1 | 1 | 1 | 1 | 1 of 1 | `grouped.go` | 1 |",
				"| **Total** | **3** | **2** | **3** | **2** | **4 of 5** | | **1** |",
				"codersdk schemas: 4, fields 6, no comment 5",
				"codersdk schemas reached only by skipped ops: 1, no comment 1 of 1",
				"codersdk schemas with a type-level description: 0",
				"other schemas: 1, no comment 1 of 1",
				"all ops 4, published 3, published with description 1, skipped with description 0",
				"tags with a tag-level description: 1 of 2",
				"unmapped codersdk types: 1 ['codersdk.OnlySkipped']",
			},
		},
		{
			name: "OrderAppendsRemainingTagsByPublishedCount",
			// Beta is not listed, so it follows Alpha.
			extra: []string{"-order", "Alpha"},
			want:  []string{"| Alpha |", "| Beta |"},
		},
		{
			name:  "TagDetailAlpha",
			extra: []string{"-tag", "Alpha"},
			want: []string{
				"## Operations with no description (1)\nGET /a  List a\n",
				"## Thin parameters (1)\nGET /a  limit (query): \"Page limit\"\n",
				"## codersdk fields with no comment (3)\n" +
					"codersdk.Nested.value  grouped.go\n" +
					"codersdk.Shared.name  shared.go\n" +
					"codersdk.Shared.nested  shared.go\n",
			},
			notWant: []string{"Search query", "POST /a", "codersdk.BetaOnly"},
		},
		{
			name:  "TagDetailBetaExcludesSharedAndSkipped",
			extra: []string{"-tag", "Beta"},
			want: []string{
				"## Operations with no description (1)\nGET /b  Get b\n",
				"## Thin parameters (1)\nGET /b  body (body): \"\"\n",
				"## codersdk fields with no comment (1)\ncodersdk.BetaOnly.flag  grouped.go\n",
			},
			notWant: []string{"Skipped", "codersdk.Shared", "codersdk.OnlySkipped"},
		},
		{
			name:    "UnknownTag",
			extra:   []string{"-tag", "Nope"},
			wantErr: `unknown tag "Nope"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			err := run(append(append([]string{}, base...), tt.extra...), &out)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			for _, w := range tt.want {
				require.Contains(t, out.String(), w)
			}
			for _, nw := range tt.notWant {
				require.NotContains(t, out.String(), nw)
			}
		})
	}
}

func TestRun_Deterministic(t *testing.T) {
	t.Parallel()

	swaggerPath, sdkDir := writeFixtures(t)
	args := []string{"-swagger", swaggerPath, "-codersdk", sdkDir}
	var first bytes.Buffer
	require.NoError(t, run(args, &first))
	for range 5 {
		var again bytes.Buffer
		require.NoError(t, run(args, &again))
		require.Equal(t, first.String(), again.String())
	}
	require.False(t, strings.Contains(first.String(), "\r"))
}

func TestRun_MissingSwagger(t *testing.T) {
	t.Parallel()

	err := run([]string{"-swagger", filepath.Join(t.TempDir(), "missing.json")}, &bytes.Buffer{})
	require.ErrorContains(t, err, "read swagger")
}
