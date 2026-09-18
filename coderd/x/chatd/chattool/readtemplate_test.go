package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
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
		AgentsAllowed:   true,
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
		AgentsAllowed:   true,
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

func TestReadTemplate_Readme(t *testing.T) {
	t.Parallel()

	// Seed the database, user, and organization once and reuse them across
	// subtests; each subtest only adds its own template (and version).
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	readTemplateInfo := func(t *testing.T, activeVersionID uuid.UUID) map[string]any {
		t.Helper()
		tmpl := dbgen.Template(t, db, database.Template{
			OrganizationID:  org.ID,
			CreatedBy:       user.ID,
			ActiveVersionID: activeVersionID,
			AgentsAllowed:   true,
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

	readTemplateInfoForReadme := func(t *testing.T, readme string) map[string]any {
		t.Helper()
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Readme:         readme,
		})
		return readTemplateInfo(t, tv.ID)
	}

	t.Run("Surfaced", func(t *testing.T) {
		t.Parallel()
		readme := "---\ndescription: Go template.\n---\n# Title\n\nUse Docker.\n"
		tmplInfo := readTemplateInfoForReadme(t, readme)
		require.Equal(t, "Title\nUse Docker.", tmplInfo["readme"])
	})

	t.Run("EmptyOmitsField", func(t *testing.T) {
		t.Parallel()
		tmplInfo := readTemplateInfoForReadme(t, " \n\t\n")
		_, ok := tmplInfo["readme"]
		require.False(t, ok, "readme should be omitted when blank")
	})

	t.Run("NotTruncatedUnderCap", func(t *testing.T) {
		t.Parallel()
		readme := "# Title\n\n" + strings.Repeat("x", 3000)
		tmplInfo := readTemplateInfoForReadme(t, readme)
		require.Equal(t, "Title\n"+strings.Repeat("x", 3000), tmplInfo["readme"])
	})

	// Images are dropped but code blocks are preserved as text (detail view).
	t.Run("DropsImagesKeepsCode", func(t *testing.T) {
		t.Parallel()
		readme := "# Setup\n\n![diagram](./a.svg)\n\nRun the installer.\n\n```sh\nmake build\n```\n\nDone.\n"
		tmplInfo := readTemplateInfoForReadme(t, readme)
		require.Equal(t, "Setup\nRun the installer.\nmake build\nDone.", tmplInfo["readme"])
	})

	// READMEs larger than the cap are truncated with a trailing ellipsis so a
	// single large document cannot dominate the response.
	t.Run("TruncatedOverCap", func(t *testing.T) {
		t.Parallel()
		readme := strings.Repeat("x", 9000)
		tmplInfo := readTemplateInfoForReadme(t, readme)
		got, ok := tmplInfo["readme"].(string)
		require.True(t, ok)
		gotRunes := []rune(got)
		require.Len(t, gotRunes, chattool.ReadTemplateReadmeMaxRunes)
		require.Equal(t, '…', gotRunes[len(gotRunes)-1])
	})

	// A template whose active version row is missing must not fail
	// read_template; the version fetch is best-effort and readme is simply
	// omitted.
	t.Run("MissingVersionOmitsField", func(t *testing.T) {
		t.Parallel()
		tmplInfo := readTemplateInfo(t, uuid.New())
		_, ok := tmplInfo["readme"]
		require.False(t, ok, "readme should be omitted when the version is missing")
	})
}

// TestReadTemplate_OwnerEvaluatedParameters covers the parameter source
// selection: owner-evaluated parameters replace the import-time rows, and
// any failure to evaluate falls back to those rows with a note saying so.
func TestReadTemplate_OwnerEvaluatedParameters(t *testing.T) {
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
		AgentsAllowed:   true,
	})
	// The import-time row records the default seen by the template
	// importer, which is what the fallback path must report.
	_ = dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{
		TemplateVersionID: tv.ID,
		Name:              "Region",
		Type:              "string",
		DefaultValue:      "us-pittsburgh",
		Mutable:           false,
		Options:           json.RawMessage(`[{"name":"Pittsburgh","value":"us-pittsburgh"},{"name":"Falkenstein","value":"eu-helsinki"}]`),
	})

	regex := "^[a-z-]+$"
	ownerRendered := []codersdk.PreviewParameter{
		{
			PreviewParameterData: codersdk.PreviewParameterData{
				Name:         "Region",
				Type:         codersdk.OptionTypeString,
				FormType:     codersdk.ParameterFormTypeRadio,
				Mutable:      false,
				DefaultValue: codersdk.NullHCLString{Value: "eu-helsinki", Valid: true},
				Options: []codersdk.PreviewParameterOption{
					{Name: "Pittsburgh", Value: codersdk.NullHCLString{Value: "us-pittsburgh", Valid: true}},
					{Name: "Falkenstein", Value: codersdk.NullHCLString{Value: "eu-helsinki", Valid: true}},
				},
				Validations: []codersdk.PreviewParameterValidation{{Regex: &regex}},
			},
			Value: codersdk.NullHCLString{Value: "eu-helsinki", Valid: true},
		},
	}

	readTemplateParams := func(t *testing.T, templateID uuid.UUID, render chattool.RenderTemplateParametersFn) (map[string]any, string) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		tool := chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{
			OwnerID:          user.ID,
			RenderParameters: render,
			Logger:           slogtest.Make(t, nil),
		})
		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-owner",
			Name:  "read_template",
			Input: `{"template_id":"` + templateID.String() + `"}`,
		})
		require.NoError(t, err)
		require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		params, ok := result["parameters"].([]any)
		require.True(t, ok)
		require.Len(t, params, 1)
		note, _ := result["parameters_note"].(string)
		return params[0].(map[string]any), note
	}
	readParams := func(t *testing.T, render chattool.RenderTemplateParametersFn) (map[string]any, string) {
		t.Helper()
		return readTemplateParams(t, tmpl.ID, render)
	}

	t.Run("OwnerDefaultsWin", func(t *testing.T) {
		t.Parallel()
		var gotOwner, gotVersion uuid.UUID
		region, note := readParams(t, func(_ context.Context, ownerID, versionID uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			gotOwner, gotVersion = ownerID, versionID
			return ownerRendered, []codersdk.FriendlyDiagnostic{{Severity: codersdk.DiagnosticSeverityWarning, Summary: "ignored"}}, nil
		})
		require.Equal(t, user.ID, gotOwner)
		require.Equal(t, tv.ID, gotVersion)
		require.Equal(t, "eu-helsinki", region["default"])
		require.Equal(t, false, region["mutable"])
		require.Equal(t, "radio", region["form_type"])
		require.Equal(t, regex, region["validation_regex"])
		opts, ok := region["options"].([]any)
		require.True(t, ok)
		require.Len(t, opts, 2)
		require.Equal(t, "eu-helsinki", opts[1].(map[string]any)["value"])
		require.Contains(t, note, "values a build for this workspace owner uses")
	})

	t.Run("ParameterErrorIsReported", func(t *testing.T) {
		t.Parallel()
		// Mirrors preview output for an owner-evaluated default outside the
		// option set: the default itself is a valid string and the error
		// is scoped to the parameter, not the top-level diagnostics.
		broken := ownerRendered[0]
		broken.DefaultValue = codersdk.NullHCLString{Value: "not-an-option", Valid: true}
		broken.Value = codersdk.NullHCLString{}
		broken.Diagnostics = []codersdk.FriendlyDiagnostic{{
			Severity: codersdk.DiagnosticSeverityError,
			Summary:  "Value must be a valid option",
			Detail:   "not-an-option is not one of the options",
		}}
		region, note := readParams(t, func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			return []codersdk.PreviewParameter{broken}, nil, nil
		})
		_, hasDefault := region["default"]
		require.False(t, hasDefault, "an errored default must not be asserted")
		require.Equal(t, `Value must be a valid option: not-an-option is not one of the options; the evaluated default "not-an-option" cannot be used, pass a value explicitly`, region["error"])
		require.Contains(t, note, "values a build for this workspace owner uses")
	})

	t.Run("RequiredWithoutValueIsNotAnError", func(t *testing.T) {
		t.Parallel()
		// preview tags required parameters rendered with no inputs with a
		// "required" error diagnostic; that must not be reported as a
		// broken default.
		required := codersdk.PreviewParameter{
			PreviewParameterData: codersdk.PreviewParameterData{
				Name:     "Region",
				Type:     codersdk.OptionTypeString,
				Required: true,
				Mutable:  true,
			},
			Diagnostics: []codersdk.FriendlyDiagnostic{{
				Severity: codersdk.DiagnosticSeverityError,
				Summary:  "Required parameter not provided",
				Detail:   "parameter value is null",
				Extra:    codersdk.DiagnosticExtra{Code: "required"},
			}},
		}
		region, note := readParams(t, func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			return []codersdk.PreviewParameter{required}, nil, nil
		})
		require.Equal(t, true, region["required"])
		_, hasDefault := region["default"]
		require.False(t, hasDefault)
		_, hasErr := region["error"]
		require.False(t, hasErr)
		require.Contains(t, note, "values a build for this workspace owner uses")
	})

	t.Run("RenderErrorFallsBack", func(t *testing.T) {
		t.Parallel()
		region, note := readParams(t, func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			return nil, nil, xerrors.New("boom")
		})
		require.Equal(t, "us-pittsburgh", region["default"])
		require.Equal(t, false, region["mutable"])
		require.Contains(t, note, "recorded at template import")
	})

	t.Run("ErrorDiagnosticFallsBack", func(t *testing.T) {
		t.Parallel()
		region, note := readParams(t, func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			return ownerRendered, []codersdk.FriendlyDiagnostic{{Severity: codersdk.DiagnosticSeverityError, Summary: "bad template"}}, nil
		})
		require.Equal(t, "us-pittsburgh", region["default"])
		require.Contains(t, note, "recorded at template import")
	})

	t.Run("NotReadyReportsImporting", func(t *testing.T) {
		t.Parallel()
		region, note := readParams(t, func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			return nil, nil, xerrors.Errorf("prepare: %w", dynamicparameters.ErrTemplateVersionNotReady)
		})
		require.Equal(t, "us-pittsburgh", region["default"])
		require.Contains(t, note, "still importing")
	})

	t.Run("ClassicFlowSkipsOwnerEvaluation", func(t *testing.T) {
		t.Parallel()
		// Builds for classic-flow templates use the import rows, so those are
		// reported as the build values without consulting the renderer.
		classicVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		classic := dbgen.Template(t, db, database.Template{
			OrganizationID:          org.ID,
			CreatedBy:               user.ID,
			ActiveVersionID:         classicVersion.ID,
			AgentsAllowed:           true,
			UseClassicParameterFlow: true,
		})
		_ = dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{
			TemplateVersionID: classicVersion.ID,
			Name:              "Region",
			Type:              "string",
			DefaultValue:      "us-pittsburgh",
		})
		region, note := readTemplateParams(t, classic.ID, func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			t.Fatal("renderer must not run for classic-flow templates")
			return nil, nil, nil
		})
		require.Equal(t, "us-pittsburgh", region["default"])
		require.Contains(t, note, "values a build for this workspace owner uses")
	})

	t.Run("BuildTimeDefaultUsesImportValue", func(t *testing.T) {
		t.Parallel()
		unknown := ownerRendered[0]
		unknown.DefaultValue = codersdk.NullHCLString{}
		unknown.Value = codersdk.NullHCLString{}
		region, note := readParams(t, func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			return []codersdk.PreviewParameter{unknown}, nil, nil
		})
		require.Equal(t, "us-pittsburgh", region["default"])
		require.Contains(t, region["default_note"], "recorded at template import")
		_, hasErr := region["error"]
		require.False(t, hasErr)
		require.Contains(t, note, "values a build for this workspace owner uses")
	})

	t.Run("NoRendererUsesImportDefaults", func(t *testing.T) {
		t.Parallel()
		region, note := readParams(t, nil)
		require.Equal(t, "us-pittsburgh", region["default"])
		require.Contains(t, note, "recorded at template import")
	})
}
