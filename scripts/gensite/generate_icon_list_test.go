package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

// TestSVGIconAttributes validates that all SVG icons have the required
// attributes: width="256", height="256", and viewBox="0 0 256 256".
func TestSVGIconAttributes(t *testing.T) {
	t.Parallel()

	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get test file location")
	testDir := filepath.Dir(testFile)
	repoRoot := filepath.Join(testDir, "..", "..")
	iconDir := filepath.Join(repoRoot, "site", "static", "icon")

	files, err := os.ReadDir(iconDir)
	require.NoError(t, err, "failed to read icon directory")

	for _, file := range files {
		if !file.Type().IsRegular() {
			continue
		}

		fileName := file.Name()
		if !strings.HasSuffix(fileName, ".svg") {
			continue
		}

		t.Run(fileName, func(t *testing.T) {
			t.Parallel()
			filePath := filepath.Join(iconDir, fileName)

			content, err := os.ReadFile(filePath)
			require.NoError(t, err, "failed to read SVG file")

			attrs, err := parseSVGAttributes(string(content))
			require.NoError(t, err, "failed to parse SVG")

			require.Equal(t, "256", attrs["width"],
				"SVG must have width=\"256\"")
			require.Equal(t, "256", attrs["height"],
				"SVG must have height=\"256\"")
			require.Equal(t, "0 0 256 256", attrs["viewBox"],
				"SVG must have viewBox=\"0 0 256 256\"")
		})
	}
}

// parseSVGAttributes parses the root SVG element and returns its attributes.
func parseSVGAttributes(content string) (map[string]string, error) {
	// Match the opening <svg> tag with optional attributes
	svgTagRegex := regexp.MustCompile(`<svg(\s+[^>]*)?>`)
	matches := svgTagRegex.FindStringSubmatch(content)
	if len(matches) == 0 {
		return nil, xerrors.New("no <svg> tag found")
	}

	var attrsStr string
	if len(matches) >= 2 && matches[1] != "" {
		attrsStr = strings.TrimSpace(matches[1])
	}

	attrs := parseAttributes(attrsStr)
	return attrs, nil
}

// parseAttributes parses a string of XML attributes into a map.
func parseAttributes(attrsStr string) map[string]string {
	attrs := make(map[string]string)
	if attrsStr == "" {
		return attrs
	}

	// Match attribute="value" or attribute='value' patterns
	attrRegex := regexp.MustCompile(`(\S+)=["']([^"']*)["']`)
	matches := attrRegex.FindAllStringSubmatch(attrsStr, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			key := strings.TrimSpace(match[1])
			value := match[2]
			attrs[key] = value
		}
	}

	return attrs
}
