package docgenenv

import (
	"errors"
	"slices"

	"golang.org/x/xerrors"
)

// PageMetadata is the per-page metadata that a generated page carries in its
// front matter. The generators read it from their overlay file, and
// BuildManifest reads it back from the page's front matter.
type PageMetadata struct {
	Description string   `yaml:"description,omitempty"`
	IconPath    string   `yaml:"icon_path,omitempty"`
	State       []string `yaml:"state,omitempty"`
}

// SectionMetadata is the PageMetadata for the generated section whose title
// matches Title.
type SectionMetadata struct {
	Title        string `yaml:"title"`
	PageMetadata `yaml:",inline"`
}

// Overlays is the decoded overlays.yml of a doc generator. Index applies to
// the generated index page, and Sections applies to generated section pages
// by title.
type Overlays struct {
	Index    PageMetadata      `yaml:"index,omitempty"`
	Sections []SectionMetadata `yaml:"sections,omitempty"`
}

// LoadOverlays decodes and checks the overlays file at path. Descriptions must
// end with a period, like every other page description, and each section
// title may appear only once.
func LoadOverlays(path string) (*Overlays, error) {
	var o Overlays
	if err := DecodeSourceFile(path, &o); err != nil {
		return nil, err
	}
	var errs []error
	checkMetadata := func(where string, p PageMetadata) {
		checkDescription(where, "description", p.Description, &errs)
	}
	checkMetadata("index", o.Index)
	seen := make(map[string]bool, len(o.Sections))
	for _, s := range o.Sections {
		switch {
		case s.Title == "":
			errs = append(errs, xerrors.Errorf("%s: section overlay is missing a title", path))
			continue
		case seen[s.Title]:
			errs = append(errs, xerrors.Errorf("%s: section %q is listed more than once", path, s.Title))
		}
		seen[s.Title] = true
		checkMetadata(s.Title, s.PageMetadata)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &o, nil
}

// Route returns the route for a generated page titled title, carrying p's
// metadata. The title comes from the sidebar, so the sidebar keeps the
// structure and the overlay keeps only metadata.
func (p PageMetadata) Route(title string) Route {
	return Route{
		Title:       title,
		Description: p.Description,
		IconPath:    p.IconPath,
		State:       slices.Clone(p.State),
	}
}
