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
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

const (
	apiSubdir       = "reference/api"
	apiIndexFile    = "index.md"
	apiIndexContent = `# API

Get started with the Coder API:

## Quickstart

Generate a token on your Coder deployment by visiting:

` + "````shell" + `
https://coder.example.com/settings/tokens
` + "````" + `

List your workspaces

` + "````shell" + `
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
	err := os.WriteFile(path.Join(apiDir, apiIndexFile), []byte(apiIndexContent), 0o644) // #nosec
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
		err = os.WriteFile(docPath, section, 0o644) // #nosec
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
		return sort.StringsAreSorted([]string{mdFiles[i].title, mdFiles[j].title})
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
		if r.Title != "API" {
			continue
		}

		var children []route
		for _, mdf := range mdFiles {
			docRoute := route{
				Title: mdf.title,
				Path:  mdf.path,
			}
			children = append(children, docRoute)
		}

		m.Routes[i].Children = children
		break
	}

	manifestFile, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		return xerrors.Errorf("json.Marshal failed: %w", err)
	}

	err = os.WriteFile(manifestPath, manifestFile, 0o644) // #nosec
	if err != nil {
		return xerrors.Errorf("can't write manifest file: %w", err)
	}
	log.Printf("Write manifest file: %dB", len(manifestFile))
	return nil
}

func extractSectionName(section []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(section))
	if !scanner.Scan() {
		return "", xerrors.Errorf("section header was expected")
	}

	header := scanner.Text()[2:] // Skip #<space>
	return strings.TrimSpace(header), nil
}

func toMdFilename(sectionName string) string {
	return nonAlphanumericRegex.ReplaceAllLiteralString(strings.ToLower(sectionName), "-") + ".md"
}
