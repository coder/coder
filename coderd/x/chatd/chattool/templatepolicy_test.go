package chattool_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplatePolicyCheck(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db := dbmock.NewMockStore(gomock.NewController(t))
	versionID := uuid.New()
	template := database.Template{ID: uuid.New(), ActiveVersionID: versionID}
	db.EXPECT().GetTemplateVersionByID(gomock.Any(), versionID).Return(database.TemplateVersion{
		ExternalAuthProviders: []byte(`[{"id":"github-security"}]`),
	}, nil).AnyTimes()

	require.Empty(t, chattool.TemplatePolicy{}.Check(ctx, db, template))
	require.Empty(t, chattool.TemplatePolicy{AllowedTemplateIDs: []uuid.UUID{template.ID}}.Check(ctx, db, template))
	require.Contains(t, chattool.TemplatePolicy{AllowedTemplateIDs: []uuid.UUID{uuid.New()}}.Check(ctx, db, template),
		"not allowed to use this template")

	require.Empty(t, chattool.TemplatePolicy{AllowedExternalAuthProviderIDs: []string{"github-security"}}.Check(ctx, db, template))
	require.Contains(t, chattool.TemplatePolicy{AllowedExternalAuthProviderIDs: []string{"github"}}.Check(ctx, db, template),
		`"github-security" external auth provider`)
}

func TestTemplatePolicyParameterRules(t *testing.T) {
	t.Parallel()

	versionID := uuid.New()
	template := database.Template{ID: uuid.New(), ActiveVersionID: versionID}
	policy := chattool.TemplatePolicy{ParameterRules: map[uuid.UUID]map[string][]string{
		template.ID: {"region": {"us-east", "eu-west"}},
	}}
	newDB := func(t *testing.T) *dbmock.MockStore {
		db := dbmock.NewMockStore(gomock.NewController(t))
		db.EXPECT().GetTemplateVersionParameters(gomock.Any(), versionID).Return([]database.TemplateVersionParameter{
			{Name: "region"}, {Name: "size"},
		}, nil).AnyTimes()
		return db
	}

	t.Run("FillsDefault", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		got, reason := policy.ApplyParameterRules(ctx, newDB(t), template, uuid.Nil, map[string]string{"size": "large"})
		require.Empty(t, reason)
		require.Equal(t, map[string]string{"region": "us-east", "size": "large"}, got)
	})

	t.Run("KeepsAllowedValue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		got, reason := policy.ApplyParameterRules(ctx, newDB(t), template, uuid.Nil, map[string]string{"region": "eu-west"})
		require.Empty(t, reason)
		require.Equal(t, map[string]string{"region": "eu-west"}, got)
	})

	t.Run("RejectsDisallowedValue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		_, reason := policy.ApplyParameterRules(ctx, newDB(t), template, uuid.Nil, map[string]string{"region": "ap-south"})
		require.Contains(t, reason, `parameter "region" to one of: "us-east", "eu-west"`)
	})

	t.Run("RejectsPresetWithDisallowedValue", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := newDB(t)
		presetID := uuid.New()
		db.EXPECT().GetPresetParametersByPresetID(gomock.Any(), presetID).Return([]database.TemplateVersionPresetParameter{
			{Name: "region", Value: "ap-south"},
		}, nil)
		_, reason := policy.ApplyParameterRules(ctx, db, template, presetID, map[string]string{"region": "us-east"})
		require.Contains(t, reason, "may not use this preset")
	})

	t.Run("PresetValueIsNotOverwritten", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db := newDB(t)
		presetID := uuid.New()
		db.EXPECT().GetPresetParametersByPresetID(gomock.Any(), presetID).Return([]database.TemplateVersionPresetParameter{
			{Name: "region", Value: "eu-west"},
		}, nil)
		got, reason := policy.ApplyParameterRules(ctx, db, template, presetID, nil)
		require.Empty(t, reason)
		require.Empty(t, got)
	})

	t.Run("OtherTemplateUnrestricted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		other := database.Template{ID: uuid.New(), ActiveVersionID: uuid.New()}
		values := map[string]string{"region": "anything"}
		got, reason := policy.ApplyParameterRules(ctx, newDB(t), other, uuid.Nil, values)
		require.Empty(t, reason)
		require.Equal(t, values, got)
		require.Empty(t, policy.CheckParameterValues(other.ID, values))
		require.NotEmpty(t, policy.CheckParameterValues(template.ID, values))
	})
}
