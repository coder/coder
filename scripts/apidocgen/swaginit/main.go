// Package main wraps swag init with Strict mode enabled and rewrites
// the generated swagger.json/docs.go so that paths mounted outside of
// the default `/api/v2` prefix (such as SCIM, OAuth2 and `.well-known`
// metadata endpoints) appear at their actual URLs in the spec.
//
// The upstream swag CLI (v1.16.2) does not expose a --strict
// flag, so warnings about duplicate routes are silently
// ignored. This wrapper calls the Go API directly with
// Strict: true, turning those warnings into hard errors.
//
// In addition, swagger 2.0 prepends a single `basePath` to every path
// in the spec.  Coder mounts most routes under `/api/v2`, but a small
// number of routes are served from the root of the HTTP server
// (e.g. `/scim/v2/...`, `/oauth2/...`, `/.well-known/...`,
// `/api/experimental/...`).  Without post-processing, `swag` either
// produces wrong "try it out" URLs in the Swagger UI for those routes
// (when `@BasePath` is `/api/v2`) or forces every `@Router` annotation
// in the codebase to be written as an absolute path (when `@BasePath`
// is `/`).  We instead keep `@Router` annotations short in the source
// and rewrite the generated spec here:
//
//   - `basePath` is set to `/`.
//   - Paths that should remain at the root keep their original value.
//   - All other paths are prefixed with `/api/v2`.
//
// This produces correct URLs for both the Swagger UI "try it out"
// feature and the generated curl examples in `docs/reference/api`.
//
// See https://github.com/coder/coder/issues/24736.
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/swaggo/swag/gen"
)

// apiV2BasePath is the base path that most Coder API routes are mounted under.
const apiV2BasePath = "/api/v2"

// rootMountedPathPrefixes are the path prefixes (in the generated
// swagger.json) that are served from the root of the Coder HTTP
// server, NOT from `/api/v2`.  Paths starting with any of these
// prefixes will be left unmodified by the rewrite step; every other
// path will be prefixed with `/api/v2`.
//
// Keep this list in sync with the route registrations in
// `coderd/coderd.go` and `enterprise/coderd/coderd.go`.
var rootMountedPathPrefixes = []string{
	"/.well-known/",
	"/oauth2/",
	"/scim/",
	"/api/experimental/",
}

// experimentalPathRewrite normalizes `@Router /experimental/...` (which
// some annotations use) to the actual mount point `/api/experimental/...`.
const experimentalPathRewrite = "/experimental/"

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	outputDir := "./coderd/apidoc"
	if d := os.Getenv("SWAG_OUTPUT_DIR"); d != "" {
		outputDir = d
	}

	err := gen.New().Build(&gen.Config{
		SearchDir:          "./coderd,./codersdk,./enterprise/coderd,./enterprise/wsproxy/wsproxysdk",
		MainAPIFile:        "coderd.go",
		OutputDir:          outputDir,
		OutputTypes:        []string{"go", "json"},
		PackageName:        "apidoc",
		ParseDependency:    1,
		Strict:             true,
		OverridesFile:      gen.DefaultOverridesFile,
		ParseGoList:        true,
		ParseDepth:         100,
		CollectionFormat:   "csv",
		Debugger:           logger,
		LeftTemplateDelim:  "{{",
		RightTemplateDelim: "}}",
	})
	if err != nil {
		log.Fatalf("swag init failed: %v", err)
	}

	if err := rewriteSwagger(outputDir); err != nil {
		log.Fatalf("rewrite swagger spec: %v", err)
	}
}

// rewriteSwagger updates the generated `swagger.json` and `docs.go`
// files in outputDir so that:
//
//   - The top-level `basePath` is `/`.
//   - Paths mounted at the root of the HTTP server (see
//     `rootMountedPathPrefixes`) keep their original value.
//   - All other paths are prefixed with `/api/v2`.
//
// `docs.go` embeds the same JSON in a Go string literal, so we apply
// the same set of rewrites to both files independently.
func rewriteSwagger(outputDir string) error {
	for _, name := range []string{"swagger.json", "docs.go"} {
		path := filepath.Join(outputDir, name)
		original, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rewritten, err := rewriteSpecBytes(original, name)
		if err != nil {
			return fmt.Errorf("rewrite %s: %w", path, err)
		}
		if err := os.WriteFile(path, rewritten, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// pathKeyRegex matches a JSON path key as emitted by swag inside the
// "paths" object of swagger.json/docs.go.  swag indents path keys with
// exactly 8 spaces and follows the closing quote with `: {`.
//
// The capture group is the raw path string (without the surrounding
// quotes).
var pathKeyRegex = regexp.MustCompile(`(?m)^( {8})"(/[^"]*)": \{`)

// basePathRegexJSON matches the `basePath` field in the swag-generated
// swagger.json.  It always sits at indent 4, with one space after the
// colon.
var basePathRegexJSON = regexp.MustCompile(`(?m)^( {4})"basePath": "/api/v2",`)

// basePathRegexDocsGo matches the `BasePath:` field of the SwaggerInfo
// Go struct that swag emits at the bottom of docs.go.
var basePathRegexDocsGo = regexp.MustCompile(`(?m)^(\tBasePath: +)"/api/v2",`)

// rewriteSpecBytes applies path-key and basePath rewrites to the given
// file contents.  `name` is used only to produce more descriptive
// error messages and to skip the JSON-only basePath rewrite when
// processing `docs.go`.
func rewriteSpecBytes(in []byte, name string) ([]byte, error) {
	// Sanity-check that swag's output still has the structure we
	// expect.  We require the JSON file to declare basePath = /api/v2
	// at the top level; if upstream swag changes its emitter we want
	// to fail loudly rather than silently producing a wrong spec.
	if name == "swagger.json" && !basePathRegexJSON.Match(in) {
		return nil, fmt.Errorf(
			"could not find expected basePath line in swagger.json. " +
				"The swag generator output format may have changed; " +
				"update scripts/apidocgen/swaginit/main.go.")
	}

	out := pathKeyRegex.ReplaceAllFunc(in, func(match []byte) []byte {
		sub := pathKeyRegex.FindSubmatch(match)
		indent := sub[1]
		key := string(sub[2])
		newKey := rewritePathKey(key)
		if newKey == key {
			return match
		}
		return []byte(fmt.Sprintf(`%s"%s": {`, indent, newKey))
	})

	// Rewrite the basePath fields so runtime consumers see the
	// updated value too.  We only touch the JSON in swagger.json (the
	// top-level `"basePath": "/api/v2"` field) and the Go literal in
	// docs.go (`BasePath: "/api/v2"`).  The {{.BasePath}} template
	// token in docs.go's docTemplate is filled at runtime from the
	// SwaggerInfo struct, so updating the struct is sufficient.
	out = basePathRegexJSON.ReplaceAll(out, []byte(`${1}"basePath": "/",`))
	out = basePathRegexDocsGo.ReplaceAll(out, []byte(`${1}"/",`))

	if !bytes.Contains(out, []byte(`"/api/v2/`)) && name == "swagger.json" {
		return nil, fmt.Errorf(
			"after rewrite, swagger.json contains no /api/v2/ paths. " +
				"This is unexpected; the rewrite logic may be broken.")
	}

	return out, nil
}

// rewritePathKey applies the prefixing rules described on
// `rewriteSwagger` to a single path key.
func rewritePathKey(key string) string {
	// `/experimental/...` is mounted at `/api/experimental/...` in
	// chi.  Canonicalize before checking the root-mounted list so we
	// don't accidentally double-prefix it.
	if strings.HasPrefix(key, experimentalPathRewrite) {
		key = "/api" + key
	}
	for _, prefix := range rootMountedPathPrefixes {
		if strings.HasPrefix(key, prefix) || key == strings.TrimSuffix(prefix, "/") {
			return key
		}
	}
	if key == "/" {
		return apiV2BasePath + "/"
	}
	return apiV2BasePath + key
}
