package chattool_test

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

func TestReadTemplate_IncludesPresets(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       user.ID,
		ActiveVersionID: tv.ID,
	})

	// Create a preset with parameters.
	const usEastLargeDesiredPrebuildInstances = 3
	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: tv.ID,
		Name:              "us-east-large",
		IsDefault:         true,
		Description:       "US East large instance",
		Icon:              "/icon/us.png",
		DesiredInstances: sql.NullInt32{
			Int32: usEastLargeDesiredPrebuildInstances,
			Valid: true,
		},
	})
	_ = dbgen.PresetParameter(t, db, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"region", "instance_type"},
		Values:                  []string{"us-east", "large"},
	})

	// Create a second preset without parameters.
	_ = dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: tv.ID,
		Name:              "empty-preset",
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	tool := chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{
		OwnerID: user.ID,
	})

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-1",
		Name:  "read_template",
		Input: `{"template_id":"` + tmpl.ID.String() + `"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))

	// Verify template info is present.
	tmplInfo, ok := result["template"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, tmpl.ID.String(), tmplInfo["id"])

	// Verify presets are present.
	presetsRaw, ok := result["presets"].([]any)
	require.True(t, ok, "expected presets in response")
	require.Len(t, presetsRaw, 2)

	// Find the preset with parameters.
	var foundPreset map[string]any
	for _, p := range presetsRaw {
		pm := p.(map[string]any)
		if pm["name"] == "us-east-large" {
			foundPreset = pm
			break
		}
	}
	require.NotNil(t, foundPreset, "expected to find us-east-large preset")
	require.Equal(t, preset.ID.String(), foundPreset["id"])
	require.Equal(t, true, foundPreset["default"])
	require.Equal(t, "US East large instance", foundPreset["description"])
	require.Equal(t, "/icon/us.png", foundPreset["icon"])
	// Prebuild count round-trips so the LLM can prefer presets
	// backed by prebuilt workspaces.
	require.EqualValues(t, usEastLargeDesiredPrebuildInstances, foundPreset["desired_prebuild_instances"])

	// Verify preset parameters.
	presetParamsRaw, ok := foundPreset["parameters"].([]any)
	require.True(t, ok)
	require.Len(t, presetParamsRaw, 2)

	paramMap := make(map[string]string)
	for _, pp := range presetParamsRaw {
		ppm := pp.(map[string]any)
		paramMap[ppm["name"].(string)] = ppm["value"].(string)
	}
	require.Equal(t, "us-east", paramMap["region"])
	require.Equal(t, "large", paramMap["instance_type"])

	// Verify the empty preset has correct defaults.
	var emptyPreset map[string]any
	for _, p := range presetsRaw {
		pm := p.(map[string]any)
		if pm["name"] == "empty-preset" {
			emptyPreset = pm
			break
		}
	}
	require.NotNil(t, emptyPreset, "expected to find empty-preset")
	require.Equal(t, false, emptyPreset["default"])
	_, hasDesc := emptyPreset["description"]
	require.False(t, hasDesc, "empty-preset should not have description")
	_, hasIcon := emptyPreset["icon"]
	require.False(t, hasIcon, "empty-preset should not have icon")
	_, hasPrebuilds := emptyPreset["desired_prebuild_instances"]
	require.False(t, hasPrebuilds, "empty-preset should not have desired_prebuild_instances")
	emptyParams, ok := emptyPreset["parameters"].([]any)
	require.True(t, ok)
	require.Empty(t, emptyParams, "empty-preset should have no parameters")
}

func TestReadTemplate_NoPresets(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       user.ID,
		ActiveVersionID: tv.ID,
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	tool := chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{
		OwnerID: user.ID,
	})

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-2",
		Name:  "read_template",
		Input: `{"template_id":"` + tmpl.ID.String() + `"}`,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))

	// Presets key should be absent when there are no presets.
	_, hasPresets := result["presets"]
	require.False(t, hasPresets, "presets key should be absent when there are none")
}

func TestReadTemplate_AgentDescription(t *testing.T) {
	t.Parallel()

	readTemplateInfo := func(t *testing.T, readme string) map[string]any {
		t.Helper()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Readme:         readme,
		})
		tmpl := dbgen.Template(t, db, database.Template{
			OrganizationID:  org.ID,
			CreatedBy:       user.ID,
			ActiveVersionID: tv.ID,
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		tool := chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{
			OwnerID: user.ID,
		})
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_template",
			Input: `{"template_id":"` + tmpl.ID.String() + `"}`,
		})
		require.NoError(t, err)
		require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		tmplInfo, ok := result["template"].(map[string]any)
		require.True(t, ok)
		return tmplInfo
	}

	t.Run("Surfaced", func(t *testing.T) {
		t.Parallel()
		tmplInfo := readTemplateInfo(t,
			"---\nagent_description: Go 1.23 with Docker and internal registry access.\n---\n# Title\n")
		require.Equal(t, "Go 1.23 with Docker and internal registry access.", tmplInfo["agent_description"])
	})

	t.Run("AbsentOmitsField", func(t *testing.T) {
		t.Parallel()
		tmplInfo := readTemplateInfo(t, "# Just a heading\n\nNo frontmatter here.\n")
		_, ok := tmplInfo["agent_description"]
		require.False(t, ok, "agent_description should be omitted when absent")
	})

	t.Run("Truncated", func(t *testing.T) {
		t.Parallel()
		tmplInfo := readTemplateInfo(t,
			"---\nagent_description: "+strings.Repeat("x", 3000)+"\n---\n# Title\n")
		got, ok := tmplInfo["agent_description"].(string)
		require.True(t, ok)
		gotRunes := []rune(got)
		require.Len(t, gotRunes, 2048)
		require.Equal(t, '…', gotRunes[len(gotRunes)-1])
	})

	// A template whose active version row is missing must not fail
	// read_template; the version fetch is best-effort and agent_description is
	// simply omitted.
	t.Run("MissingVersionOmitsField", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		tmpl := dbgen.Template(t, db, database.Template{
			OrganizationID:  org.ID,
			CreatedBy:       user.ID,
			ActiveVersionID: uuid.New(),
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		tool := chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{
			OwnerID: user.ID,
		})
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "read_template",
			Input: `{"template_id":"` + tmpl.ID.String() + `"}`,
		})
		require.NoError(t, err)
		require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		tmplInfo, ok := result["template"].(map[string]any)
		require.True(t, ok)
		_, ok = tmplInfo["agent_description"]
		require.False(t, ok, "agent_description should be omitted when the version is missing")
	})
}
