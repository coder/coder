package agentskills

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"

	"cdr.dev/slog/v3"
	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

// searchPath defines a directory to scan and its slug prefix for fallback naming.
type searchPath struct {
	dir        string // relative to homeDir, e.g. ".coder/skills"
	slugPrefix string // e.g. "coder-skills"
}

// searchPaths is computed at init time because filepath.Join is not a constant expression.
var searchPaths = []searchPath{
	{dir: filepath.Join(".coder", "skills"), slugPrefix: "coder-skills"},
	{dir: filepath.Join(".claude", "skills"), slugPrefix: "claude-skills"},
}

// skillFrontmatter represents the YAML frontmatter of a SKILL.md file.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// discoverSkills scans search paths for SKILL.md files and returns
// deduplicated, sorted skill metadata.
func discoverSkills(logger slog.Logger, homeDir string) []Skill {
	seen := make(map[string]Skill)

	for _, sp := range searchPaths {
		dir := filepath.Join(homeDir, sp.dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				logger.Warn(context.Background(), "failed to read skills directory",
					slog.F("path", dir),
					slog.Error(err),
				)
			}
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			skillFile := filepath.Join(dir, entry.Name(), skillFileName)
			raw, err := os.ReadFile(skillFile)
			if err != nil {
				if !os.IsNotExist(err) {
					logger.Warn(context.Background(), "failed to read skill file",
						slog.F("path", skillFile),
						slog.Error(err),
					)
				}
				continue
			}

			fm := parseFrontmatter(logger, skillFile, raw)
			name := fm.Name
			if name == "" {
				name = sp.slugPrefix + "-" + entry.Name()
			}

			// Dedup: first insert wins (.coder searched before .claude)
			if _, exists := seen[name]; exists {
				continue
			}

			seen[name] = Skill{
				Name:        name,
				Description: fm.Description,
				Path:        skillFile,
			}
		}
	}

	skills := make([]Skill, 0, len(seen))
	for _, s := range seen {
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills
}

// parseFrontmatter extracts YAML frontmatter from a SKILL.md file.
// Returns zero-value struct if no frontmatter is found or parsing fails.
func parseFrontmatter(logger slog.Logger, filePath string, raw []byte) skillFrontmatter {
	// Frontmatter must start with "---\n"
	if !bytes.HasPrefix(raw, []byte("---\n")) && !bytes.HasPrefix(raw, []byte("---\r\n")) {
		return skillFrontmatter{}
	}

	// Find the closing "---"
	rest := raw[4:] // skip opening "---\n"
	if bytes.HasPrefix(raw, []byte("---\r\n")) {
		rest = raw[5:]
	}
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return skillFrontmatter{}
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		logger.Warn(context.Background(), "failed to parse skill frontmatter",
			slog.F("path", filePath),
			slog.Error(err),
		)
		return skillFrontmatter{}
	}
	return fm
}
