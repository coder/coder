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
	"github.com/coder/coder/v2/scripts/docgenenv"
)

const (
	apiSubdir    = "reference/api"
	apiIndexFile = "index.md"
	// apiIndexBody is the index page content below its front matter and the
	// generated-content banner. The front matter is generated from the "REST
	// API" manifest route (see writeDocs) so the index mirrors the manifest like
	// every other generated page.
	apiIndexBody = `Get started with the Coder API:

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

## Request size limits

An endpoint that accepts a JSON body reads at most 4 MiB of it. Endpoints that
take larger payloads set a higher limit of their own: ` + "`POST /api/v2/files`" + `
accepts 100 MiB, for example. A body that exceeds the limit that applies to it
is answered with ` + "`413 Payload Too Large`" + `, and the response names the limit
that was exceeded:

` + "````json" + `
{
  "message": "Request body too large.",
  "detail": "Maximum request body size is 4194304 bytes."
}
` + "````" + `

The limits are fixed. There is no deployment option that raises them.

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

	manifestPath := path.Join(docsDirectory, "manifest.json")
	m, err := docgenenv.LoadManifest(manifestPath)
	if err != nil {
		return err
	}

	// Resolve the REST API route once. Both the page front matter and the
	// regenerated manifest routes read from this single traversal, so the
	// metadata source and the rewrite target can't drift apart.
	restAPI := m.FindRoute("Reference", "REST API")
	if restAPI == nil {
		return xerrors.Errorf("could not find REST API route in manifest %q", manifestPath)
	}

	// Index existing REST API child routes by title so their curated metadata
	// (description, state, icon_path) flows into both the page front matter and
	// the regenerated manifest routes.
	existingByTitle := make(map[string]docgenenv.Route)
	for _, child := range restAPI.Children {
		existingByTitle[child.Title] = child
	}

	apiDir := path.Join(docsDirectory, apiSubdir)

	// The index page mirrors the "REST API" route's own curated metadata
	// (title/description/icon_path) rather than a hardcoded title, so its front
	// matter matches the manifest like every other generated page.
	indexRoute := *restAPI
	indexRoute.Children = nil
	indexContent := append([]byte(docgenenv.GeneratedHeader(indexRoute)), []byte(apiIndexBody)...)
	if err := atomicwrite.File(path.Join(apiDir, apiIndexFile), indexContent); err != nil {
		return xerrors.Errorf(`can't write the index file: %w`, err)
	}

	type mdFile struct {
		title string
		path  string
	}
	var mdFiles []mdFile

	// Write .md files for grouped API methods (Templates, Workspaces, etc.)
	for _, section := range sections {
		sectionName, err := extractSectionName(section)
		if err != nil {
			return xerrors.Errorf("can't extract section name: %w", err)
		}
		log.Printf("Write section: %s", sectionName)

		// Carry the manifest route's curated metadata into the front matter.
		r := existingByTitle[sectionName]
		r.Title = sectionName

		mdFilename := toMdFilename(sectionName)
		docPath := path.Join(apiDir, mdFilename)
		if err := atomicwrite.File(docPath, prependGeneratedHeader(section, r)); err != nil {
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

	// Update manifest.json. Generated routes overwrite Title and Path;
	// existing state/description/icon_path are preserved (keyed by title) so
	// callouts like `state: ["experimental"]` survive regeneration. restAPI
	// aliases m, so replacing its children updates the manifest in place.
	var children []docgenenv.Route
	for _, mdf := range mdFiles {
		docRoute := docgenenv.Route{
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
	restAPI.Children = children

	manifestFile, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return xerrors.Errorf("json.Marshal failed: %w", err)
	}

	if err := atomicwrite.File(manifestPath, manifestFile); err != nil {
		return xerrors.Errorf("can't write manifest file: %w", err)
	}
	log.Printf("Write manifest file: %dB", len(manifestFile))
	return nil
}

func extractSectionName(section []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(section))
	if !scanner.Scan() {
		// Scan returns false on EOF or error. A first line past
		// bufio.Scanner's token limit surfaces only in Err(); report it as a
		// scanning error rather than mislabeling it a missing header.
		if err := scanner.Err(); err != nil {
			return "", xerrors.Errorf("scanning section: %w", err)
		}
		return "", xerrors.Errorf("section header was expected")
	}

	header := scanner.Text()
	name, ok := strings.CutPrefix(header, "# ")
	if !ok {
		return "", xerrors.Errorf("section header %q must start with %q", header, "# ")
	}
	return strings.TrimSpace(name), nil
}

func toMdFilename(sectionName string) string {
	return nonAlphanumericRegex.ReplaceAllLiteralString(strings.ReplaceAll(strings.ToLower(sectionName), " ", ""), "-") + ".md"
}

// prependGeneratedHeader replaces the leading "# {name}" heading of a raw API
// section with r's generated-page header (front matter plus the shared
// generated-content banner). Callers pass sections that have already cleared
// extractSectionName, whose fail-fast on a missing "# " heading is the
// load-bearing guarantee. The prefix check here is a defensive backstop: if a
// section without the heading ever reached this function, it keeps the body
// intact instead of dropping the first real content line.
func prependGeneratedHeader(section []byte, r docgenenv.Route) []byte {
	body := section
	if bytes.HasPrefix(section, []byte("# ")) {
		if _, rest, found := bytes.Cut(section, []byte{'\n'}); found {
			body = rest
		} else {
			body = nil
		}
	}
	body = bytes.TrimLeft(body, "\r\n")
	return append([]byte(docgenenv.GeneratedHeader(r)), body...)
}
