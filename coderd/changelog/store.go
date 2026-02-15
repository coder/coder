package changelog

import (
	"bytes"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// EntryMeta holds the YAML frontmatter of a changelog entry.
type EntryMeta struct {
	Version string `yaml:"version" json:"version"`
	Title   string `yaml:"title"   json:"title"`
	Date    string `yaml:"date"    json:"date"`
	Summary string `yaml:"summary" json:"summary"`
	Image   string `yaml:"image"   json:"image"`
}

// Entry is a parsed changelog entry with body markdown.
type Entry struct {
	EntryMeta
	Content string `json:"content"`
}

// Store provides access to embedded changelog entries.
type Store struct {
	once    sync.Once
	entries []Entry
	byVer   map[string]*Entry
	err     error
}

// NewStore creates a Store that lazily parses the embedded FS.
func NewStore() *Store {
	return &Store{}
}

func (s *Store) init() {
	s.once.Do(func() {
		s.byVer = make(map[string]*Entry)

		files, err := fs.ReadDir(FS, "entries")
		if err != nil {
			s.err = xerrors.Errorf("read entries dir: %w", err)
			return
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}

			data, err := fs.ReadFile(FS, path.Join("entries", f.Name()))
			if err != nil {
				s.err = xerrors.Errorf("read %s: %w", f.Name(), err)
				return
			}

			entry, err := ParseEntry(data)
			if err != nil {
				s.err = xerrors.Errorf("parse %s: %w", f.Name(), err)
				return
			}

			s.entries = append(s.entries, *entry)
			s.byVer[entry.Version] = &s.entries[len(s.entries)-1]
		}

		// Sort by date descending, then version descending as a tiebreaker.
		sort.Slice(s.entries, func(i, j int) bool {
			if s.entries[i].Date != s.entries[j].Date {
				return s.entries[i].Date > s.entries[j].Date
			}
			vi := "v" + s.entries[i].Version
			vj := "v" + s.entries[j].Version
			return semver.Compare(vi, vj) > 0
		})

		// Rebuild map pointers after sort.
		for i := range s.entries {
			s.byVer[s.entries[i].Version] = &s.entries[i]
		}
	})
}

// List returns all entries sorted by date desc.
func (s *Store) List() ([]Entry, error) {
	s.init()
	if s.err != nil {
		return nil, s.err
	}
	return s.entries, nil
}

// Get returns a single entry by version string (e.g. "2.30").
func (s *Store) Get(version string) (*Entry, error) {
	s.init()
	if s.err != nil {
		return nil, s.err
	}
	e, ok := s.byVer[version]
	if !ok {
		return nil, xerrors.Errorf("changelog entry not found for version %s", version)
	}
	return e, nil
}

// Has reports whether an entry exists for the given version.
func (s *Store) Has(version string) bool {
	s.init()
	e, _ := s.Get(version)
	return e != nil
}

// ParseEntry parses a changelog entry markdown file (YAML frontmatter + body).
func ParseEntry(data []byte) (*Entry, error) {
	// Split frontmatter from body. Format: ---\nyaml\n---\nmarkdown
	const delimiter = "---"
	trimmed := bytes.TrimSpace(data)
	if !bytes.HasPrefix(trimmed, []byte(delimiter)) {
		return nil, xerrors.New("missing frontmatter delimiter")
	}

	rest := trimmed[len(delimiter):]
	idx := bytes.Index(rest, []byte("\n"+delimiter))
	if idx < 0 {
		return nil, xerrors.New("missing closing frontmatter delimiter")
	}

	fmData := rest[:idx]
	body := rest[idx+len("\n"+delimiter):]

	var meta EntryMeta
	if err := yaml.Unmarshal(fmData, &meta); err != nil {
		return nil, xerrors.Errorf("unmarshal frontmatter: %w", err)
	}
	if meta.Version == "" {
		return nil, xerrors.New("frontmatter missing required 'version' field")
	}

	return &Entry{
		EntryMeta: meta,
		Content:   strings.TrimSpace(string(body)),
	}, nil
}
