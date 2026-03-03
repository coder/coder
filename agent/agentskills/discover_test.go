package agentskills

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
)

// createSkillFile is a helper that creates a SKILL.md file under
// homeDir/searchDir/skillName/SKILL.md and returns its absolute path.
func createSkillFile(t *testing.T, homeDir, searchDir, skillName, content string) string {
	t.Helper()
	dir := filepath.Join(homeDir, searchDir, skillName)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "SKILL.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestDiscoverSkills_SingleCoderSkill(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	skillPath := createSkillFile(t, home, ".coder/skills", "code-review",
		"---\nname: code-review\ndescription: Reviews code\n---\n\n# Body content\n")

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 1)
	require.Equal(t, "code-review", skills[0].Name)
	require.Equal(t, "Reviews code", skills[0].Description)
	require.Equal(t, skillPath, skills[0].Path)
	require.True(t, filepath.IsAbs(skills[0].Path))
}

func TestDiscoverSkills_MultipleSkills_AlphabeticalOrder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	createSkillFile(t, home, ".coder/skills", "zebra",
		"---\nname: zebra\ndescription: Z skill\n---\n")
	createSkillFile(t, home, ".coder/skills", "alpha",
		"---\nname: alpha\ndescription: A skill\n---\n")

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 2)
	require.Equal(t, "alpha", skills[0].Name)
	require.Equal(t, "zebra", skills[1].Name)
}

func TestDiscoverSkills_ClaudePathFallback(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	createSkillFile(t, home, ".claude/skills", "review",
		"---\nname: review\ndescription: Claude review\n---\n")

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 1)
	require.Equal(t, "review", skills[0].Name)
	require.Equal(t, "Claude review", skills[0].Description)
}

func TestDiscoverSkills_BothPaths_Merged(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	createSkillFile(t, home, ".coder/skills", "lint",
		"---\nname: lint\ndescription: Linter\n---\n")
	createSkillFile(t, home, ".claude/skills", "format",
		"---\nname: format\ndescription: Formatter\n---\n")

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 2)
	require.Equal(t, "format", skills[0].Name)
	require.Equal(t, "lint", skills[1].Name)
}

func TestDiscoverSkills_Dedup_CoderWins(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	createSkillFile(t, home, ".coder/skills", "review",
		"---\nname: review\ndescription: Coder review\n---\n")
	createSkillFile(t, home, ".claude/skills", "review",
		"---\nname: review\ndescription: Claude review\n---\n")

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 1)
	require.Equal(t, "review", skills[0].Name)
	require.Equal(t, "Coder review", skills[0].Description)
}

func TestDiscoverSkills_NoFrontmatter_SlugFallback(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	createSkillFile(t, home, ".coder/skills", "foo",
		"# Just a markdown body\n\nNo frontmatter here.\n")

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 1)
	require.Equal(t, "coder-skills-foo", skills[0].Name)
	require.Equal(t, "", skills[0].Description)
}

func TestDiscoverSkills_ClaudePath_SlugFallback(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	createSkillFile(t, home, ".claude/skills", "bar",
		"# Just a markdown body\n\nNo frontmatter here.\n")

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 1)
	require.Equal(t, "claude-skills-bar", skills[0].Name)
}

func TestDiscoverSkills_DirWithoutSkillMd_Skipped(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	// Create an empty directory with no SKILL.md
	emptyDir := filepath.Join(home, ".coder", "skills", "empty")
	require.NoError(t, os.MkdirAll(emptyDir, 0o755))

	// Create a valid skill
	createSkillFile(t, home, ".coder/skills", "valid",
		"---\nname: valid\ndescription: A valid skill\n---\n")

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 1)
	require.Equal(t, "valid", skills[0].Name)
}

func TestDiscoverSkills_NoSearchPaths_EmptyResult(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	skills := discoverSkills(logger, home)

	require.NotNil(t, skills)
	require.Empty(t, skills)
}

func TestDiscoverSkills_MalformedYAML_WarnAndFallback(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	createSkillFile(t, home, ".coder/skills", "bad",
		"---\n: broken\n---\n")

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 1)
	require.Equal(t, "coder-skills-bad", skills[0].Name)
}

func TestDiscoverSkills_BodyNotIncluded(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	bigBody := strings.Repeat("A", 10*1024)
	content := "---\nname: big\ndescription: Big skill\n---\n\n" + bigBody
	createSkillFile(t, home, ".coder/skills", "big", content)

	skills := discoverSkills(logger, home)

	require.Len(t, skills, 1)
	require.Equal(t, "big", skills[0].Name)
	require.Equal(t, "Big skill", skills[0].Description)
	// The Skill struct should only carry name, description, path — no body.
	b, err := json.Marshal(skills[0])
	require.NoError(t, err)
	require.NotContains(t, string(b), bigBody)
}

func TestSkillsEndpoint_ReturnsJSON(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	createSkillFile(t, home, ".coder/skills", "test-skill",
		"---\nname: test-skill\ndescription: Test skill\n---\n")

	api := NewAPI(logger, home)
	handler := api.Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var skills []Skill
	err := json.NewDecoder(rec.Body).Decode(&skills)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	require.Equal(t, "test-skill", skills[0].Name)
	require.Equal(t, "Test skill", skills[0].Description)
}

func TestSkillsEndpoint_EmptySkills_ReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	logger := slogtest.Make(t, nil)

	api := NewAPI(logger, home)
	handler := api.Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	body := strings.TrimSpace(rec.Body.String())
	require.Equal(t, "[]", body)
}
