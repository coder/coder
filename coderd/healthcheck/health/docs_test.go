package health_test

import (
	"bufio"
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/healthcheck/health"
)

// healthCheckDocsPath is the page that health.Message.URL links to.
var healthCheckDocsPath = filepath.Join("..", "..", "..", "docs", "admin", "monitoring", "health-check.md")

// TestCodesDocumented ensures every health code constant has a heading in the
// health check docs whose anchor matches the fragment of health.Message.URL.
// Operators follow that URL from the health page and CLI output, so a missing
// heading is a broken link.
func TestCodesDocumented(t *testing.T) {
	t.Parallel()

	codes := codeConstants(t)
	require.NotEmpty(t, codes, "no Code constants found in model.go")

	anchors := markdownAnchors(t, healthCheckDocsPath)

	for name, code := range codes {
		u, err := url.Parse(health.Message{Code: health.Code(code)}.URL(""))
		require.NoError(t, err)
		if _, ok := anchors[u.Fragment]; !ok {
			t.Errorf("health code %s (%s) has no heading with anchor #%s in %s; add a \"### %s\" section", code, name, u.Fragment, healthCheckDocsPath, code)
		}
	}
}

// codeConstants returns the string value of every constant whose name starts
// with "Code" in model.go, keyed by constant name. Constants are read from
// source so that new codes are covered without updating this test, including
// codes declared without the Code type.
func codeConstants(t *testing.T) map[string]string {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "model.go", nil, 0)
	require.NoError(t, err)

	codes := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Code") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				require.True(t, ok && lit.Kind == token.STRING, "constant %s must be a string literal", name.Name)
				value, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				codes[name.Name] = value
			}
		}
	}
	return codes
}

var (
	headingPattern     = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)
	anchorStripPattern = regexp.MustCompile(`[^a-z0-9 _-]`)
)

// markdownAnchors returns the GitHub-style anchors of every heading in the
// Markdown file at path, ignoring lines inside fenced code blocks.
func markdownAnchors(t *testing.T, path string) map[string]struct{} {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	anchors := map[string]struct{}{}
	inFence := false
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := headingPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		anchor := anchorStripPattern.ReplaceAllString(strings.ToLower(m[1]), "")
		anchors[strings.ReplaceAll(anchor, " ", "-")] = struct{}{}
	}
	require.NoError(t, scanner.Err())
	return anchors
}
