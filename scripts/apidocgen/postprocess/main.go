package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scripts/atomicwrite"
)

const (
	apiSubdir       = "reference/api"
	apiIndexFile    = "index.md"
	apiIndexContent = `---
title: REST API
---

Get started with the Coder API:

## Quickstart

Generate a token on your Coder deployment by visiting:

` + "````txt" + `
https://coder.example.com/settings/tokens
` + "````" + `

List your workspaces

` + "````sh" + `
# CLI
curl https://coder.example.com/api/v2/workspaces?q=owner:me \
-H "Coder-Session-Token: <your-token>"
` + "````" + `

## Use cases

See some common [use cases](../../reference/index.md#use-cases) for the REST API.

## Sections

<children>
  This page is rendered on https://coder.com/docs/reference/api. Refer to the other documents in the ` + "`api/`" + ` directory.
</children>
`
)

var (
	docsDirectory  string
	inMdFileSingle string

	sectionSeparator     = []byte("<!-- APIDOCGEN: BEGIN SECTION -->\n")
	nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9 ]+`)
)

func main() {
	log.Println("Postprocess API docs")

	flag.StringVar(&docsDirectory, "docs-directory", "../../docs", "Path to Coder docs directory")
	flag.StringVar(&inMdFileSingle, "in-md-file-single", "", "Path to single Markdown file, output from widdershins.js")
	flag.Parse()

	if inMdFileSingle == "" {
		flag.Usage()
		log.Fatal("missing value for in-md-file-single")
	}

	sections, err := loadMarkdownSections()
	if err != nil {
		log.Fatal("can't load markdown sections: ", err)
	}

	err = prepareDocsDirectory()
	if err != nil {
		log.Fatal("can't prepare docs directory: ", err)
	}

	err = writeDocs(sections)
	if err != nil {
		log.Fatal("can't write docs directory: ", err)
	}

	log.Println("Done")
}

func loadMarkdownSections() ([][]byte, error) {
	log.Printf("Read the md-file-single: %s", inMdFileSingle)
	mdFile, err := os.ReadFile(inMdFileSingle)
	if err != nil {
		return nil, xerrors.Errorf("can't read the md-file-single: %w", err)
	}
	log.Printf("Read %dB", len(mdFile))

	sections := bytes.Split(mdFile, sectionSeparator)
	if len(sections) < 2 {
		return nil, xerrors.Errorf("At least 1 section is expected: %w", err)
	}
	sections = sections[1:] // Skip the first element which is the empty byte array
	log.Printf("Loaded %d sections", len(sections))
	return sections, nil
}

func prepareDocsDirectory() error {
	log.Println("Prepare docs directory")

	apiPath := path.Join(docsDirectory, apiSubdir)

	err := os.RemoveAll(apiPath)
	if err != nil {
		return xerrors.Errorf(`os.RemoveAll failed for "%s": %w`, apiPath, err)
	}

	err = os.MkdirAll(apiPath, 0o755)
	if err != nil {
		return xerrors.Errorf(`os.MkdirAll failed for "%s": %w`, apiPath, err)
	}
	return nil
}

func writeDocs(sections [][]byte) error {
	log.Println("Write docs to destination")

	apiDir := path.Join(docsDirectory, apiSubdir)
	err := atomicwrite.File(path.Join(apiDir, apiIndexFile), []byte(apiIndexContent))
	if err != nil {
		return xerrors.Errorf(`can't write the index file: %w`, err)
	}

	type mdFile struct {
		title string
		path  string
	}
	var mdFiles []mdFile

	// Write .md files for grouped API method (Templates, Workspaces, etc.)
	for _, section := range sections {
		sectionName, err := extractSectionName(section)
		if err != nil {
			return xerrors.Errorf("can't extract section name: %w", err)
		}
		log.Printf("Write section: %s", sectionName)

		mdFilename := toMdFilename(sectionName)
		docPath := path.Join(apiDir, mdFilename)
		err = atomicwrite.File(docPath, section)
		if err != nil {
			return xerrors.Errorf(`can't write doc file "%s": %w`, docPath, err)
		}
		mdFiles = append(mdFiles, mdFile{
			title: sectionName,
			path:  "./" + path.Join(apiSubdir, mdFilename),
		})
	}

	// Sort API pages
	// The "General" section is expected to be always first.
	sort.Slice(mdFiles, func(i, j int) bool {
		if mdFiles[i].title == "General" {
			return true // "General" < ... - sorted
		}
		if mdFiles[j].title == "General" {
			return false // ... < "General" - not sorted
		}
		return slices.IsSorted([]string{mdFiles[i].title, mdFiles[j].title})
	})

	// Update manifest.json
	type route struct {
		Title       string   `json:"title,omitempty"`
		Description string   `json:"description,omitempty"`
		Path        string   `json:"path,omitempty"`
		IconPath    string   `json:"icon_path,omitempty"`
		State       []string `json:"state,omitempty"`
		Children    []route  `json:"children,omitempty"`
	}

	type manifest struct {
		Versions []string `json:"versions,omitempty"`
		Routes   []route  `json:"routes,omitempty"`
	}

	manifestPath := path.Join(docsDirectory, "manifest.json")
	manifestFile, err := os.ReadFile(manifestPath)
	if err != nil {
		return xerrors.Errorf("can't read manifest file: %w", err)
	}
	log.Printf("Read manifest file: %dB", len(manifestFile))

	var m manifest
	err = json.Unmarshal(manifestFile, &m)
	if err != nil {
		return xerrors.Errorf("json.Unmarshal failed: %w", err)
	}

	for i, r := range m.Routes {
		if r.Title != "Reference" {
			continue
		}
		for j, child := range r.Children {
			if child.Title != "REST API" {
				continue
			}

			// Preserve existing state and description on children, keyed by
			// title, so that callouts like `state: ["experimental"]` survive
			// regeneration. Generated routes always overwrite Title and Path.
			existingByTitle := make(map[string]route, len(child.Children))
			for _, existing := range child.Children {
				existingByTitle[existing.Title] = existing
			}

			var children []route
			for _, mdf := range mdFiles {
				docRoute := route{
					Title: mdf.title,
					Path:  mdf.path,
				}
				if existing, ok := existingByTitle[mdf.title]; ok {
					docRoute.State = existing.State
					docRoute.Description = existing.Description
					docRoute.IconPath = existing.IconPath
				}
				children = append(children, docRoute)
			}

			m.Routes[i].Children[j].Children = children
			break
		}
		break
	}

	manifestFile, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		return xerrors.Errorf("json.Marshal failed: %w", err)
	}

	err = atomicwrite.File(manifestPath, manifestFile)
	if err != nil {
		return xerrors.Errorf("can't write manifest file: %w", err)
	}
	log.Printf("Write manifest file: %dB", len(manifestFile))
	return nil
}

func extractSectionName(section []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(section))

	// Generated sections can be preceded by blank lines; capture the first
	// non-blank line as the header.
	var first string
	for scanner.Scan() {
		if line := scanner.Text(); strings.TrimSpace(line) != "" {
			first = line
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", xerrors.Errorf("scanning section: %w", err)
	}

	// Parse front-matter block: --- ... title: <value> ... ---
	if first == "---" {
		for scanner.Scan() {
			line := scanner.Text()
			if line == "---" {
				break // closing delimiter reached without finding a title
			}
			if after, ok := strings.CutPrefix(line, "title:"); ok {
				// Strip optional surrounding YAML quotes.
				title := strings.Trim(strings.TrimSpace(after), `"'`)
				if title != "" {
					return title, nil
				}
			}
		}
		return "", xerrors.Errorf("front-matter block has no non-empty %q key; section starts: %q", "title:", sectionPreview(section))
	}

	// Fallback: legacy "# Name" heading.
	if after, ok := strings.CutPrefix(first, "# "); ok {
		return strings.TrimSpace(after), nil
	}

	return "", xerrors.Errorf("section header not found: want a front-matter title: or a leading '# ' heading, got %q", first)
}

// sectionPreview returns a compact, single-line excerpt of a section's leading
// content for error messages, so a malformed section is identifiable from the
// make gen output.
func sectionPreview(section []byte) string {
	const maxRunes = 120
	preview := strings.Join(strings.Fields(string(section)), " ")
	if runes := []rune(preview); len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return preview
}

func toMdFilename(sectionName string) string {
	return nonAlphanumericRegex.ReplaceAllLiteralString(strings.ReplaceAll(strings.ToLower(sectionName), " ", ""), "-") + ".md"
}
