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

	// newTemplate creates an agents-allowed template whose active version
	// carries the given import-time parameter rows.
	newTemplate := func(t *testing.T, seed database.Template, rows ...database.TemplateVersionParameter) database.Template {
		t.Helper()
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
		})
		seed.OrganizationID = org.ID
		seed.CreatedBy = user.ID
		seed.ActiveVersionID = tv.ID
		seed.AgentsAllowed = true
		tmpl := dbgen.Template(t, db, seed)
		for _, row := range rows {
			row.TemplateVersionID = tv.ID
			_ = dbgen.TemplateVersionParameter(t, db, row)
		}
		return tmpl
	}
	// regionRow is the import-time row for the Region parameter. It records
	// the default seen by the template importer, which is what the fallback
	// path must report.
	regionRow := database.TemplateVersionParameter{
		Name:         "Region",
		Type:         "string",
		DefaultValue: "us-pittsburgh",
		Mutable:      false,
		Options:      json.RawMessage(`[{"name":"Pittsburgh","value":"us-pittsburgh"},{"name":"Falkenstein","value":"eu-helsinki"}]`),
	}
	tmpl := newTemplate(t, database.Template{}, regionRow)

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

	// readAll runs read_template and returns the parameters by name plus the
	// parameters_note.
	readAll := func(t *testing.T, templateID uuid.UUID, render chattool.RenderTemplateParametersFn) (map[string]map[string]any, string) {
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

		var result struct {
			Parameters []map[string]any `json:"parameters"`
			Note       string           `json:"parameters_note"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		byName := make(map[string]map[string]any, len(result.Parameters))
		for _, p := range result.Parameters {
			byName[p["name"].(string)] = p
		}
		require.Len(t, byName, len(result.Parameters), "parameter names must be unique")
		return byName, result.Note
	}
	// readParams reads the shared template and returns its Region parameter.
	readParams := func(t *testing.T, render chattool.RenderTemplateParametersFn) (map[string]any, string) {
		t.Helper()
		params, note := readAll(t, tmpl.ID, render)
		require.Len(t, params, 1)
		return params["Region"], note
	}
	renderStatic := func(params ...codersdk.PreviewParameter) chattool.RenderTemplateParametersFn {
		return func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			return params, nil, nil
		}
	}

	t.Run("OwnerDefaultsWin", func(t *testing.T) {
		t.Parallel()
		var gotOwner, gotVersion uuid.UUID
		region, note := readParams(t, func(_ context.Context, ownerID, versionID uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			gotOwner, gotVersion = ownerID, versionID
			return ownerRendered, []codersdk.FriendlyDiagnostic{{Severity: codersdk.DiagnosticSeverityWarning, Summary: "ignored"}}, nil
		})
		require.Equal(t, user.ID, gotOwner)
		require.Equal(t, tmpl.ActiveVersionID, gotVersion)
		require.Equal(t, "eu-helsinki", region["default"])
		require.Equal(t, false, region["mutable"])
		require.Equal(t, "radio", region["form_type"])
		require.Equal(t, regex, region["validation_regex"])
		opts, ok := region["options"].([]any)
		require.True(t, ok)
		require.Len(t, opts, 2)
		require.Equal(t, "eu-helsinki", opts[1].(map[string]any)["value"])
		require.NotContains(t, region, "default_note")
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
		region, note := readParams(t, renderStatic(broken))
		require.NotContains(t, region, "default", "an errored default must not be asserted")
		require.Equal(t, `Value must be a valid option: not-an-option is not one of the options; the evaluated default "not-an-option" cannot be used, pass a value explicitly`, region["error"])
		require.Contains(t, note, "values a build for this workspace owner uses")
	})

	t.Run("RequiredWithoutValueIsNotAnError", func(t *testing.T) {
		t.Parallel()
		// preview tags required parameters rendered with no inputs with a
		// "required" error diagnostic; that must not be reported as a
		// broken default, and the import default must not stand in.
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
		region, note := readParams(t, renderStatic(required))
		require.Equal(t, true, region["required"])
		require.NotContains(t, region, "default")
		require.NotContains(t, region, "error")
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
		classic := newTemplate(t, database.Template{UseClassicParameterFlow: true}, regionRow)
		params, note := readAll(t, classic.ID, func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			t.Fatal("renderer must not run for classic-flow templates")
			return nil, nil, nil
		})
		require.Equal(t, "us-pittsburgh", params["Region"]["default"])
		require.Contains(t, note, "values a build for this workspace owner uses")
	})

	t.Run("UnresolvedDefaultIsNotFilledFromImport", func(t *testing.T) {
		t.Parallel()
		// Preview renders a default that depends on data.coder_provisioner
		// and a default that evaluates to null for this owner identically:
		// Valid is false and there is no diagnostic. The import row cannot
		// tell them apart, so its value must not stand in.
		unknown := ownerRendered[0]
		unknown.DefaultValue = codersdk.NullHCLString{}
		unknown.Value = codersdk.NullHCLString{}
		region, note := readParams(t, renderStatic(unknown))
		require.NotContains(t, region, "default", "import default must not stand in for an unevaluated one")
		require.Contains(t, region["default_note"], "no default could be evaluated")
		require.NotContains(t, region, "error")
		require.Contains(t, note, "values a build for this workspace owner uses")
	})

	t.Run("IncompleteRenderKeepsModuleParameters", func(t *testing.T) {
		t.Parallel()
		// preview drops every parameter declared by a module it cannot load
		// and reports that as a module_not_loaded warning, not an error.
		// provisionerd still resolves the module at build time, so the
		// import row is the best estimate for those parameters.
		withModule := newTemplate(t, database.Template{}, regionRow, database.TemplateVersionParameter{
			Name:         "jetbrains_ide",
			Type:         "string",
			DefaultValue: "GO",
		})
		params, note := readAll(t, withModule.ID, func(context.Context, uuid.UUID, uuid.UUID) ([]codersdk.PreviewParameter, []codersdk.FriendlyDiagnostic, error) {
			return ownerRendered, []codersdk.FriendlyDiagnostic{{
				Severity: codersdk.DiagnosticSeverityWarning,
				Summary:  "Module not loaded. Did you run `terraform init`?",
				Extra:    codersdk.DiagnosticExtra{Code: "module_not_loaded"},
			}}, nil
		})
		require.Len(t, params, 2)
		require.Equal(t, "eu-helsinki", params["Region"]["default"], "rendered parameters keep the owner default")
		require.Equal(t, "GO", params["jetbrains_ide"]["default"], "module parameter recovered from the import row")
		require.Contains(t, params["jetbrains_ide"]["default_note"], "recorded at template import")
		require.Contains(t, note, "values a build for this workspace owner uses")
	})

	t.Run("UnknownOptionValuesAreOmitted", func(t *testing.T) {
		t.Parallel()
		// An option value that depends on data preview cannot see, such as
		// data.coder_provisioner attributes, renders as invalid and preview
		// flags the parameter. The option is left out rather than shown with
		// an empty value or replaced from the import row, and the error
		// tells the model to pass a value explicitly.
		unknownOption := ownerRendered[0]
		unknownOption.Options = []codersdk.PreviewParameterOption{
			{Name: "Pittsburgh", Value: codersdk.NullHCLString{Value: "us-pittsburgh", Valid: true}},
			{Name: "Native (" + user.Username + ")", Value: codersdk.NullHCLString{}},
		}
		unknownOption.Diagnostics = []codersdk.FriendlyDiagnostic{{
			Severity: codersdk.DiagnosticSeverityError,
			Summary:  "Parameter contains 1 invalid options",
			Detail:   "The set of options cannot be resolved, and use of the parameter is limited.",
		}}
		region, _ := readParams(t, renderStatic(unknownOption))
		require.Contains(t, region["error"], "invalid options")
		opts, ok := region["options"].([]any)
		require.True(t, ok)
		require.Len(t, opts, 1, "only options with a known value are listed")
		require.Equal(t, "us-pittsburgh", opts[0].(map[string]any)["value"])
	})

	t.Run("ValidationIsNotFilledFromImport", func(t *testing.T) {
		t.Parallel()
		// Preview returns a nil bound both for one that evaluates to null
		// for this owner and for one it could not evaluate, so the import
		// row's bound must not stand in for a missing one.
		constrained := regionRow
		constrained.Name = "cpu"
		constrained.Type = "number"
		constrained.DefaultValue = "4"
		constrained.Options = nil
		constrained.ValidationMin = sql.NullInt32{Int32: 2, Valid: true}
		constrained.ValidationMax = sql.NullInt32{Int32: 16, Valid: true}
		withValidation := newTemplate(t, database.Template{}, constrained)
		maxCPU := int64(16)
		params, _ := readAll(t, withValidation.ID, renderStatic(codersdk.PreviewParameter{
			PreviewParameterData: codersdk.PreviewParameterData{
				Name:         "cpu",
				Type:         codersdk.OptionTypeNumber,
				Mutable:      true,
				DefaultValue: codersdk.NullHCLString{Value: "4", Valid: true},
				Validations:  []codersdk.PreviewParameterValidation{{Max: &maxCPU}},
			},
			Value: codersdk.NullHCLString{Value: "4", Valid: true},
		}))
		cpu := params["cpu"]
		require.Equal(t, "4", cpu["default"])
		require.EqualValues(t, 16, cpu["validation_max"], "evaluated bounds come from the render")
		require.NotContains(t, cpu, "validation_min", "import bound must not stand in for a null one")
		require.NotContains(t, cpu, "default_note")
	})

	t.Run("NoRendererUsesImportDefaults", func(t *testing.T) {
		t.Parallel()
		region, note := readParams(t, nil)
		require.Equal(t, "us-pittsburgh", region["default"])
		require.Contains(t, note, "recorded at template import")
	})
}
