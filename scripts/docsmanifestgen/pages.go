package main

import (
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/scripts/docgenenv"
)

// exemptPages are pages that the folder check skips. The first group stays out
// of the sidebar on purpose and should become drafts once the docs support
// them. The second group is listed by a sidebar in another folder and should
// move when the folders match the sidebar.
var exemptPages = []string{
	"AGENTS.md",
	"admin/integrations/opentofu.md",
	"templates/extending-templates/external-auth.md",

	"install/validate/scale-coder.md",
}

// unlistedPagesIn reports every Markdown page under docsDir that its own
// folder's sidebar does not list, which includes every page in a folder with
// no sidebar file. A page counts as listed by the sidebar of its folder or by the sidebar
// of the folder named like the page ("migrate.md" and "migrate/"), which is
// how a parent page sits beside its children. Pages under a generated folder
// are listed by the generator, so they are skipped. Call it only after
// docgenenv.BuildManifest succeeds.
func unlistedPagesIn(docsDir string) []error {
	s, err := docgenenv.LoadSources(docsDir)
	if err != nil {
		return []error{err}
	}
	var errs []error
	err = filepath.WalkDir(docsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != docsDir && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(docsDir, p)
		if err != nil {
			return err
		}
		page := filepath.ToSlash(rel)
		if slices.Contains(exemptPages, page) || underAny(page, s.GeneratedDirs) {
			return nil
		}
		dir := path.Dir(page)
		sidebarDir, listed := s.ListedBy[page]
		switch {
		case listed && (sidebarDir == dir || sidebarDir == strings.TrimSuffix(page, ".md")):
		case listed:
			errs = append(errs, xerrors.Errorf("%s: listed in %s, not in the sidebar of its own folder", page, path.Join(sidebarDir, docgenenv.SidebarFile)))
		default:
			errs = append(errs, xerrors.Errorf("%s: not listed in %s", page, path.Join(dir, docgenenv.SidebarFile)))
		}
		return nil
	})
	if err != nil {
		errs = append(errs, xerrors.Errorf("walk %q: %w", docsDir, err))
	}
	return errs
}

func underAny(page string, dirs []string) bool {
	for _, d := range dirs {
		if strings.HasPrefix(page, d+"/") {
			return true
		}
	}
	return false
}
