package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/templatebuilder"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateBuilderBases(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderBases(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Bases)
		require.Len(t, resp.Bases, len(templatebuilder.BaseTemplateIDs()))

		basesByID := make(map[string]codersdk.TemplateBuilderBase, len(resp.Bases))
		for _, b := range resp.Bases {
			basesByID[b.ID] = b
		}

		type baseSpec struct {
			id           string
			expectedOS   string
			expectedVars []string
			hasVariables bool
		}

		specs := []baseSpec{
			{
				id:           "docker",
				expectedOS:   "linux",
				hasVariables: true,
				expectedVars: []string{"container_image"},
			},
			{
				id:           "kubernetes",
				expectedOS:   "linux",
				hasVariables: true,
				expectedVars: []string{"container_image", "namespace", "use_kubeconfig"},
			},
			{
				id:           "aws-linux",
				expectedOS:   "linux",
				hasVariables: false,
			},
			{
				id:           "aws-windows",
				expectedOS:   "windows",
				hasVariables: false,
			},
			{
				id:           "gcp-windows",
				expectedOS:   "windows",
				hasVariables: true,
				expectedVars: []string{"project_id"},
			},
		}

		for _, spec := range specs {
			b, ok := basesByID[spec.id]
			require.True(t, ok, "base %q missing from response", spec.id)
			require.NotEmpty(t, b.Name, "base %q should have a name", spec.id)
			require.NotEmpty(t, b.Icon, "base %q should have an icon", spec.id)
			require.Equal(t, spec.expectedOS, b.OS, "base %q OS mismatch", spec.id)
			require.NotNil(t, b.Variables, "base %q should have non-nil variables slice", spec.id)

			if spec.hasVariables {
				require.NotEmpty(t, b.Variables, "base %q should have variables", spec.id)
				varNames := make(map[string]bool, len(b.Variables))
				for _, v := range b.Variables {
					varNames[v.Name] = true
				}
				for _, expected := range spec.expectedVars {
					require.True(t, varNames[expected],
						"base %q should have variable %q", spec.id, expected)
				}
			} else {
				require.Empty(t, b.Variables, "base %q should have no variables", spec.id)
			}
		}
	})

	t.Run("Sorted", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderBases(ctx)
		require.NoError(t, err)

		for i := 1; i < len(resp.Bases); i++ {
			require.LessOrEqual(t, resp.Bases[i-1].Name, resp.Bases[i].Name,
				"bases should be sorted by name")
		}
	})

	t.Run("DisabledReturns404", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.TemplateBuilder.Disabled = true

		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateBuilderBases(ctx)
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

func TestTemplateBuilderModules(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderModules(ctx, "")
		require.NoError(t, err)
		require.NotEmpty(t, resp.Modules)

		for _, m := range resp.Modules {
			require.NotEmpty(t, m.ID)
			require.NotEmpty(t, m.Version)
		}
	})

	t.Run("FilteredByBase", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderModules(ctx, "docker")
		require.NoError(t, err)

		for _, m := range resp.Modules {
			if len(m.CompatibleOS) > 0 {
				require.Contains(t, m.CompatibleOS, "linux",
					"module %q should be compatible with linux when filtered by docker base", m.ID)
			}
		}
	})

	t.Run("ComputedVariablesExcluded", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		resp, err := client.TemplateBuilderModules(ctx, "")
		require.NoError(t, err)

		// The embedded code-server module has agent_id with computed=true.
		// It must not appear in the API response.
		var found bool
		for _, m := range resp.Modules {
			if m.ID == "code-server" {
				found = true
				for _, v := range m.Variables {
					require.NotEqual(t, "agent_id", v.Name,
						"computed variable agent_id must not appear in API response")
				}
			}
		}
		require.True(t, found, "code-server module must be in the catalog")
	})

	t.Run("UnknownBaseReturns400", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateBuilderModules(ctx, "nonexistent")
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("DisabledReturns404", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.TemplateBuilder.Disabled = true

		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateBuilderModules(ctx, "")
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

func TestTemplateBuilderSession(t *testing.T) {
	t.Parallel()

	t.Run("WizardEntry", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.TemplateBuilderSession(ctx, codersdk.TemplateBuilderSessionRequest{
			SessionID: uuid.New(),
			EventType: codersdk.TemplateBuilderSessionEventWizardEntry,
		})
		require.NoError(t, err)
	})

	t.Run("ComposeCompletion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.TemplateBuilderSession(ctx, codersdk.TemplateBuilderSessionRequest{
			SessionID:       uuid.New(),
			EventType:       codersdk.TemplateBuilderSessionEventComposeCompletion,
			BaseTemplateID:  "docker",
			ModuleIDs:       []string{"code-server", "git-clone"},
			DurationSeconds: 42.5,
			Success:         true,
		})
		require.NoError(t, err)
	})

	t.Run("MissingSessionID", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.TemplateBuilderSession(ctx, codersdk.TemplateBuilderSessionRequest{
			EventType: codersdk.TemplateBuilderSessionEventWizardEntry,
		})
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("InvalidEventType", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.TemplateBuilderSession(ctx, codersdk.TemplateBuilderSessionRequest{
			EventType: "invalid_event",
		})
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("DisabledReturns404", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.TemplateBuilder.Disabled = true

		client := coderdtest.New(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		_ = coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.TemplateBuilderSession(ctx, codersdk.TemplateBuilderSessionRequest{
			EventType: codersdk.TemplateBuilderSessionEventWizardEntry,
		})
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("MemberCannotSubmit", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		admin := coderdtest.CreateFirstUser(t, client)

		memberClient, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := memberClient.TemplateBuilderSession(ctx, codersdk.TemplateBuilderSessionRequest{
			EventType: codersdk.TemplateBuilderSessionEventWizardEntry,
		})
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})
}

func TestTemplateBuilderCreateTemplateFailureTelemetry(t *testing.T) {
	t.Parallel()

	t.Run("ComposeInvalid", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		reporter := newFakeTelemetryReporter(ctx, t, 100)
		client := coderdtest.New(t, &coderdtest.Options{
			TelemetryReporter: reporter,
		})
		user := coderdtest.CreateFirstUser(t, client)

		sessionID := uuid.New()
		_, err := client.TemplateBuilderCreateTemplate(ctx, codersdk.TemplateBuilderCreateTemplateRequest{
			SessionID:      sessionID,
			BaseTemplateID: "nonexistent",
			Modules: []codersdk.TemplateBuilderComposeModule{
				{ID: "code-server"},
			},
			OrganizationID: user.OrganizationID,
			Name:           "compose-invalid",
		})
		require.Error(t, err)

		event := receiveTemplateBuilderSession(ctx, t, reporter)
		require.Equal(t, telemetry.TemplateBuilderSessionEventBuildFailure, event.EventType)
		require.Equal(t, telemetry.TemplateBuilderFailureComposeInvalid, event.FailureReason)
		// The session ID doubles as the event ID so the failure joins to the
		// wizard_entry event of the same visit.
		require.Equal(t, sessionID, event.ID)
		require.Equal(t, user.UserID, event.UserID)
		require.Equal(t, "nonexistent", event.BaseTemplateID)
		require.Equal(t, []string{"code-server"}, event.ModuleIDs)
		require.False(t, event.Success)
	})

	t.Run("MissingBaseTemplateID", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		reporter := newFakeTelemetryReporter(ctx, t, 100)
		client := coderdtest.New(t, &coderdtest.Options{
			TelemetryReporter: reporter,
		})
		user := coderdtest.CreateFirstUser(t, client)

		_, err := client.TemplateBuilderCreateTemplate(ctx, codersdk.TemplateBuilderCreateTemplateRequest{
			OrganizationID: user.OrganizationID,
			Name:           "missing-base",
		})
		require.Error(t, err)

		event := receiveTemplateBuilderSession(ctx, t, reporter)
		require.Equal(t, telemetry.TemplateBuilderSessionEventBuildFailure, event.EventType)
		require.Equal(t, telemetry.TemplateBuilderFailureInvalidRequest, event.FailureReason)
		// Callers without a wizard session still produce a usable row.
		require.NotEqual(t, uuid.Nil, event.ID)
	})

	t.Run("NameConflict", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		reporter := newFakeTelemetryReporter(ctx, t, 100)
		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{
			Database:          db,
			Pubsub:            ps,
			TelemetryReporter: reporter,
		})
		user := coderdtest.CreateFirstUser(t, client)

		existing := dbgen.Template(t, db, database.Template{
			OrganizationID: user.OrganizationID,
			CreatedBy:      user.UserID,
			Name:           "taken-name",
		})

		_, err := client.TemplateBuilderCreateTemplate(ctx, codersdk.TemplateBuilderCreateTemplateRequest{
			BaseTemplateID: "docker",
			OrganizationID: user.OrganizationID,
			Name:           existing.Name,
		})
		require.Error(t, err)

		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())

		event := receiveTemplateBuilderSession(ctx, t, reporter)
		require.Equal(t, telemetry.TemplateBuilderSessionEventBuildFailure, event.EventType)
		require.Equal(t, telemetry.TemplateBuilderFailureNameConflict, event.FailureReason)
		require.Equal(t, "docker", event.BaseTemplateID)
	})
}

// receiveTemplateBuilderSession drains snapshots until one carries a template
// builder session event. Unrelated snapshots, such as those reported while
// creating the first user, arrive on the same channel.
func receiveTemplateBuilderSession(ctx context.Context, t *testing.T, reporter *fakeTelemetryReporter) telemetry.TemplateBuilderSession {
	t.Helper()
	for {
		snapshot := testutil.TryReceive(ctx, t, reporter.snapshots)
		if len(snapshot.TemplateBuilderSessions) == 0 {
			continue
		}
		require.Len(t, snapshot.TemplateBuilderSessions, 1)
		return snapshot.TemplateBuilderSessions[0]
	}
}
