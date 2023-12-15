package coderd_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplate(t *testing.T) {
	t.Parallel()

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
	})
}

func TestPostTemplateByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		owner := coderdtest.CreateFirstUser(t, client)
		// By default, everyone in the org can read the template.
		user, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		auditor.ResetLogs()

		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)

		expected := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		got, err := user.Template(ctx, expected.ID)
		require.NoError(t, err)

		assert.Equal(t, expected.Name, got.Name)
		assert.Equal(t, expected.Description, got.Description)

		require.Len(t, auditor.AuditLogs(), 3)
		assert.Equal(t, database.AuditActionCreate, auditor.AuditLogs()[0].Action)
		assert.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[1].Action)
		assert.Equal(t, database.AuditActionCreate, auditor.AuditLogs()[2].Action)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:      template.Name,
			VersionID: version.ID,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("DefaultTTLTooLow", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:             "testing",
			VersionID:        version.ID,
			DefaultTTLMillis: ptr.Ref(int64(-1)),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, err.Error(), "default_ttl_ms: Must be a positive integer")
	})

	t.Run("NoDefaultTTL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		got, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:             "testing",
			VersionID:        version.ID,
			DefaultTTLMillis: ptr.Ref(int64(0)),
		})
		require.NoError(t, err)
		require.Zero(t, got.DefaultTTLMillis)
	})

	t.Run("DisableEveryone", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		owner := coderdtest.CreateFirstUser(t, client)
		user, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, nil)

		expected := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(request *codersdk.CreateTemplateRequest) {
			request.DisableEveryoneGroupAccess = true
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := user.Template(ctx, expected.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, uuid.New(), codersdk.CreateTemplateRequest{
			Name:      "test",
			VersionID: uuid.New(),
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
		require.Contains(t, err.Error(), "Try logging in using 'coder login <url>'.")
	})

	t.Run("AllowUserScheduling", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			var setCalled int64
			client := coderdtest.New(t, &coderdtest.Options{
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						atomic.AddInt64(&setCalled, 1)
						require.False(t, options.UserAutostartEnabled)
						require.False(t, options.UserAutostopEnabled)
						template.AllowUserAutostart = options.UserAutostartEnabled
						template.AllowUserAutostop = options.UserAutostopEnabled
						return template, nil
					},
				},
			})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
				Name:               "testing",
				VersionID:          version.ID,
				AllowUserAutostart: ptr.Ref(false),
				AllowUserAutostop:  ptr.Ref(false),
			})
			require.NoError(t, err)

			require.EqualValues(t, 1, atomic.LoadInt64(&setCalled))
			require.False(t, got.AllowUserAutostart)
			require.False(t, got.AllowUserAutostop)
		})

		t.Run("IgnoredUnlicensed", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
				Name:               "testing",
				VersionID:          version.ID,
				AllowUserAutostart: ptr.Ref(false),
				AllowUserAutostop:  ptr.Ref(false),
			})
			require.NoError(t, err)
			// ignored and use AGPL defaults
			require.True(t, got.AllowUserAutostart)
			require.True(t, got.AllowUserAutostop)
		})
	})

	t.Run("NoVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:      "test",
			VersionID: uuid.New(),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("AutostopRequirement", func(t *testing.T) {
		t.Parallel()

		t.Run("None", func(t *testing.T) {
			t.Parallel()

			var setCalled int64
			client := coderdtest.New(t, &coderdtest.Options{
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						atomic.AddInt64(&setCalled, 1)
						assert.Zero(t, options.AutostopRequirement.DaysOfWeek)
						assert.Zero(t, options.AutostopRequirement.Weeks)

						err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
							ID:                            template.ID,
							UpdatedAt:                     dbtime.Now(),
							AllowUserAutostart:            options.UserAutostartEnabled,
							AllowUserAutostop:             options.UserAutostopEnabled,
							DefaultTTL:                    int64(options.DefaultTTL),
							UseMaxTtl:                     options.UseMaxTTL,
							MaxTTL:                        int64(options.MaxTTL),
							AutostopRequirementDaysOfWeek: int16(options.AutostopRequirement.DaysOfWeek),
							AutostopRequirementWeeks:      options.AutostopRequirement.Weeks,
							FailureTTL:                    int64(options.FailureTTL),
							TimeTilDormant:                int64(options.TimeTilDormant),
							TimeTilDormantAutoDelete:      int64(options.TimeTilDormantAutoDelete),
						})
						if !assert.NoError(t, err) {
							return database.Template{}, err
						}

						return db.GetTemplateByID(ctx, template.ID)
					},
				},
			})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
				Name:                "testing",
				VersionID:           version.ID,
				AutostopRequirement: nil,
			})
			require.NoError(t, err)

			require.False(t, got.UseMaxTTL) // default
			require.EqualValues(t, 1, atomic.LoadInt64(&setCalled))
			require.Empty(t, got.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 1, got.AutostopRequirement.Weeks)
		})

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			var setCalled int64
			client := coderdtest.New(t, &coderdtest.Options{
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						atomic.AddInt64(&setCalled, 1)
						assert.EqualValues(t, 0b00110000, options.AutostopRequirement.DaysOfWeek)
						assert.EqualValues(t, 2, options.AutostopRequirement.Weeks)

						err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
							ID:                            template.ID,
							UpdatedAt:                     dbtime.Now(),
							AllowUserAutostart:            options.UserAutostartEnabled,
							AllowUserAutostop:             options.UserAutostopEnabled,
							DefaultTTL:                    int64(options.DefaultTTL),
							UseMaxTtl:                     options.UseMaxTTL,
							MaxTTL:                        int64(options.MaxTTL),
							AutostopRequirementDaysOfWeek: int16(options.AutostopRequirement.DaysOfWeek),
							AutostopRequirementWeeks:      options.AutostopRequirement.Weeks,
							FailureTTL:                    int64(options.FailureTTL),
							TimeTilDormant:                int64(options.TimeTilDormant),
							TimeTilDormantAutoDelete:      int64(options.TimeTilDormantAutoDelete),
						})
						if !assert.NoError(t, err) {
							return database.Template{}, err
						}

						return db.GetTemplateByID(ctx, template.ID)
					},
				},
			})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
				Name:      "testing",
				VersionID: version.ID,
				AutostopRequirement: &codersdk.TemplateAutostopRequirement{
					// wrong order
					DaysOfWeek: []string{"saturday", "friday"},
					Weeks:      2,
				},
			})
			require.NoError(t, err)

			require.EqualValues(t, 1, atomic.LoadInt64(&setCalled))
			require.False(t, got.UseMaxTTL)
			require.Equal(t, []string{"friday", "saturday"}, got.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 2, got.AutostopRequirement.Weeks)

			got, err = client.Template(ctx, got.ID)
			require.NoError(t, err)
			require.False(t, got.UseMaxTTL)
			require.Equal(t, []string{"friday", "saturday"}, got.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 2, got.AutostopRequirement.Weeks)
		})

		t.Run("IgnoredUnlicensed", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
				Name:      "testing",
				VersionID: version.ID,
				AutostopRequirement: &codersdk.TemplateAutostopRequirement{
					DaysOfWeek: []string{"friday", "saturday"},
					Weeks:      2,
				},
			})
			require.NoError(t, err)
			// ignored and use AGPL defaults
			require.False(t, got.UseMaxTTL)
			require.Empty(t, got.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 1, got.AutostopRequirement.Weeks)
		})
	})

	t.Run("BothMaxTTLAndAutostopRequirement", func(t *testing.T) {
		t.Parallel()

		// Fake template schedule store is unneeded for this test since the
		// route fails before it is called.
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:         "testing",
			VersionID:    version.ID,
			MaxTTLMillis: ptr.Ref(24 * time.Hour.Milliseconds()),
			AutostopRequirement: &codersdk.TemplateAutostopRequirement{
				DaysOfWeek: []string{"friday", "saturday"},
				Weeks:      2,
			},
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "max_ttl_ms")
	})
}

func TestTemplatesByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		templates, err := client.TemplatesByOrganization(ctx, user.OrganizationID)
		require.NoError(t, err)
		require.NotNil(t, templates)
		require.Len(t, templates, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		templates, err := client.TemplatesByOrganization(ctx, user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, templates, 1)
	})
	t.Run("ListMultiple", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		version2 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version2.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		templates, err := client.TemplatesByOrganization(ctx, user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, templates, 2)
	})
}

func TestTemplateByOrganizationAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateByName(ctx, user.OrganizationID, "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateByName(ctx, user.OrganizationID, template.Name)
		require.NoError(t, err)
	})
}

func TestPatchTemplateMeta(t *testing.T) {
	t.Parallel()

	t.Run("Modified", func(t *testing.T) {
		t.Parallel()

		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		req := codersdk.UpdateTemplateMeta{
			Name:                         "new-template-name",
			DisplayName:                  "Displayed Name 456",
			Description:                  "lorem ipsum dolor sit amet et cetera",
			Icon:                         "/icon/new-icon.png",
			DefaultTTLMillis:             12 * time.Hour.Milliseconds(),
			AllowUserCancelWorkspaceJobs: false,
		}
		// It is unfortunate we need to sleep, but the test can fail if the
		// updatedAt is too close together.
		time.Sleep(time.Millisecond * 5)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.Name, updated.Name)
		assert.Equal(t, req.DisplayName, updated.DisplayName)
		assert.Equal(t, req.Description, updated.Description)
		assert.Equal(t, req.Icon, updated.Icon)
		assert.Equal(t, req.DefaultTTLMillis, updated.DefaultTTLMillis)
		assert.False(t, req.AllowUserCancelWorkspaceJobs)

		// Extra paranoid: did it _really_ happen?
		updated, err = client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.Name, updated.Name)
		assert.Equal(t, req.DisplayName, updated.DisplayName)
		assert.Equal(t, req.Description, updated.Description)
		assert.Equal(t, req.Icon, updated.Icon)
		assert.Equal(t, req.DefaultTTLMillis, updated.DefaultTTLMillis)
		assert.False(t, req.AllowUserCancelWorkspaceJobs)

		require.Len(t, auditor.AuditLogs(), 5)
		assert.Equal(t, database.AuditActionWrite, auditor.AuditLogs()[4].Action)
	})

	t.Run("AGPL_Deprecated", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: false})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		// It is unfortunate we need to sleep, but the test can fail if the
		// updatedAt is too close together.
		time.Sleep(time.Millisecond * 5)

		req := codersdk.UpdateTemplateMeta{
			DeprecationMessage: ptr.Ref("APGL cannot deprecate"),
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		// AGPL cannot deprecate, expect no change
		assert.False(t, updated.Deprecated)
		assert.Empty(t, updated.DeprecationMessage)
	})

	// AGPL cannot deprecate, but it can be unset
	t.Run("AGPL_Unset_Deprecated", func(t *testing.T) {
		t.Parallel()

		owner, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: false})
		user := coderdtest.CreateFirstUser(t, owner)
		client, tplAdmin := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID, rbac.RoleTemplateAdmin())
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		// It is unfortunate we need to sleep, but the test can fail if the
		// updatedAt is too close together.
		time.Sleep(time.Millisecond * 5)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		// nolint:gocritic // Setting up unit test data
		err := db.UpdateTemplateAccessControlByID(dbauthz.As(ctx, coderdtest.AuthzUserSubject(tplAdmin, user.OrganizationID)), database.UpdateTemplateAccessControlByIDParams{
			ID:                   template.ID,
			RequireActiveVersion: false,
			Deprecated:           "Some deprecated message",
		})
		require.NoError(t, err)

		// Check that it is deprecated
		got, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		require.NotEmpty(t, got.DeprecationMessage, "template is deprecated to start")
		require.True(t, got.Deprecated, "template is deprecated to start")

		req := codersdk.UpdateTemplateMeta{
			DeprecationMessage: ptr.Ref(""),
		}

		updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.False(t, updated.Deprecated)
		assert.Empty(t, updated.DeprecationMessage)
	})

	t.Run("NoDefaultTTL", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
		})
		// It is unfortunate we need to sleep, but the test can fail if the
		// updatedAt is too close together.
		time.Sleep(time.Millisecond * 5)

		req := codersdk.UpdateTemplateMeta{
			DefaultTTLMillis: 0,
		}

		// We're too fast! Sleep so we can be sure that updatedAt is greater
		time.Sleep(time.Millisecond * 5)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)

		// Extra paranoid: did it _really_ happen?
		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.DefaultTTLMillis, updated.DefaultTTLMillis)
		assert.Empty(t, updated.DeprecationMessage)
		assert.False(t, updated.Deprecated)
	})

	t.Run("DefaultTTLTooLow", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
		})
		// It is unfortunate we need to sleep, but the test can fail if the
		// updatedAt is too close together.
		time.Sleep(time.Millisecond * 5)

		req := codersdk.UpdateTemplateMeta{
			DefaultTTLMillis: -1,
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.ErrorContains(t, err, "default_ttl_ms: Must be a positive integer")

		// Ensure no update occurred
		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Equal(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, updated.DefaultTTLMillis, template.DefaultTTLMillis)
		assert.Empty(t, updated.DeprecationMessage)
		assert.False(t, updated.Deprecated)
	})

	t.Run("MaxTTL", func(t *testing.T) {
		t.Parallel()

		const (
			defaultTTL = 1 * time.Hour
			maxTTL     = 24 * time.Hour
		)

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			var setCalled int64
			client := coderdtest.New(t, &coderdtest.Options{
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						if atomic.AddInt64(&setCalled, 1) == 2 {
							require.Equal(t, maxTTL, options.MaxTTL)
						}

						err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
							ID:                            template.ID,
							UpdatedAt:                     dbtime.Now(),
							AllowUserAutostart:            options.UserAutostartEnabled,
							AllowUserAutostop:             options.UserAutostopEnabled,
							DefaultTTL:                    int64(options.DefaultTTL),
							MaxTTL:                        int64(options.MaxTTL),
							UseMaxTtl:                     options.UseMaxTTL,
							AutostopRequirementDaysOfWeek: int16(options.AutostopRequirement.DaysOfWeek),
							AutostopRequirementWeeks:      options.AutostopRequirement.Weeks,
							FailureTTL:                    int64(options.FailureTTL),
							TimeTilDormant:                int64(options.TimeTilDormant),
							TimeTilDormantAutoDelete:      int64(options.TimeTilDormantAutoDelete),
						})
						if !assert.NoError(t, err) {
							return database.Template{}, err
						}

						return db.GetTemplateByID(ctx, template.ID)
					},
				},
			})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
				Name:                         template.Name,
				DisplayName:                  template.DisplayName,
				Description:                  template.Description,
				Icon:                         template.Icon,
				DefaultTTLMillis:             0,
				MaxTTLMillis:                 maxTTL.Milliseconds(),
				AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
			})
			require.NoError(t, err)

			require.EqualValues(t, 2, atomic.LoadInt64(&setCalled))
			require.EqualValues(t, 0, got.DefaultTTLMillis)
			require.Equal(t, maxTTL.Milliseconds(), got.MaxTTLMillis)
			require.Empty(t, got.DeprecationMessage)
			require.False(t, got.Deprecated)
		})

		t.Run("DefaultTTLBigger", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			_, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
				Name:                         template.Name,
				DisplayName:                  template.DisplayName,
				Description:                  template.Description,
				Icon:                         template.Icon,
				DefaultTTLMillis:             (maxTTL * 2).Milliseconds(),
				MaxTTLMillis:                 maxTTL.Milliseconds(),
				AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
			})
			require.Error(t, err)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
			require.Len(t, sdkErr.Validations, 1)
			require.Equal(t, "default_ttl_ms", sdkErr.Validations[0].Field)
			require.Contains(t, sdkErr.Validations[0].Detail, "Must be less than or equal to max_ttl_ms")
		})

		t.Run("IgnoredUnlicensed", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
				Name:                         template.Name,
				DisplayName:                  template.DisplayName,
				Description:                  template.Description,
				Icon:                         template.Icon,
				DefaultTTLMillis:             defaultTTL.Milliseconds(),
				MaxTTLMillis:                 maxTTL.Milliseconds(),
				AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
			})
			require.NoError(t, err)
			require.Equal(t, defaultTTL.Milliseconds(), got.DefaultTTLMillis)
			require.Zero(t, got.MaxTTLMillis)
			require.Empty(t, got.DeprecationMessage)
			require.False(t, got.Deprecated)
		})
	})

	t.Run("CleanupTTLs", func(t *testing.T) {
		t.Parallel()

		const (
			failureTTL               = 7 * 24 * time.Hour
			inactivityTTL            = 180 * 24 * time.Hour
			timeTilDormantAutoDelete = 360 * 24 * time.Hour
		)

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			var setCalled int64
			client := coderdtest.New(t, &coderdtest.Options{
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						if atomic.AddInt64(&setCalled, 1) == 2 {
							require.Equal(t, failureTTL, options.FailureTTL)
							require.Equal(t, inactivityTTL, options.TimeTilDormant)
							require.Equal(t, timeTilDormantAutoDelete, options.TimeTilDormantAutoDelete)
						}
						template.FailureTTL = int64(options.FailureTTL)
						template.TimeTilDormant = int64(options.TimeTilDormant)
						template.TimeTilDormantAutoDelete = int64(options.TimeTilDormantAutoDelete)
						return template, nil
					},
				},
			})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.FailureTTLMillis = ptr.Ref(0 * time.Hour.Milliseconds())
				ctr.TimeTilDormantMillis = ptr.Ref(0 * time.Hour.Milliseconds())
				ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref(0 * time.Hour.Milliseconds())
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
				Name:                           template.Name,
				DisplayName:                    template.DisplayName,
				Description:                    template.Description,
				Icon:                           template.Icon,
				DefaultTTLMillis:               0,
				AutostopRequirement:            &template.AutostopRequirement,
				AllowUserCancelWorkspaceJobs:   template.AllowUserCancelWorkspaceJobs,
				FailureTTLMillis:               failureTTL.Milliseconds(),
				TimeTilDormantMillis:           inactivityTTL.Milliseconds(),
				TimeTilDormantAutoDeleteMillis: timeTilDormantAutoDelete.Milliseconds(),
			})
			require.NoError(t, err)

			require.EqualValues(t, 2, atomic.LoadInt64(&setCalled))
			require.Equal(t, failureTTL.Milliseconds(), got.FailureTTLMillis)
			require.Equal(t, inactivityTTL.Milliseconds(), got.TimeTilDormantMillis)
			require.Equal(t, timeTilDormantAutoDelete.Milliseconds(), got.TimeTilDormantAutoDeleteMillis)
		})

		t.Run("IgnoredUnlicensed", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.FailureTTLMillis = ptr.Ref(0 * time.Hour.Milliseconds())
				ctr.TimeTilDormantMillis = ptr.Ref(0 * time.Hour.Milliseconds())
				ctr.TimeTilDormantAutoDeleteMillis = ptr.Ref(0 * time.Hour.Milliseconds())
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
				Name:                           template.Name,
				DisplayName:                    template.DisplayName,
				Description:                    template.Description,
				Icon:                           template.Icon,
				DefaultTTLMillis:               template.DefaultTTLMillis,
				AutostopRequirement:            &template.AutostopRequirement,
				AllowUserCancelWorkspaceJobs:   template.AllowUserCancelWorkspaceJobs,
				FailureTTLMillis:               failureTTL.Milliseconds(),
				TimeTilDormantMillis:           inactivityTTL.Milliseconds(),
				TimeTilDormantAutoDeleteMillis: timeTilDormantAutoDelete.Milliseconds(),
			})
			require.NoError(t, err)
			require.Zero(t, got.FailureTTLMillis)
			require.Zero(t, got.TimeTilDormantMillis)
			require.Zero(t, got.TimeTilDormantAutoDeleteMillis)
			require.Empty(t, got.DeprecationMessage)
			require.False(t, got.Deprecated)
		})
	})

	t.Run("AllowUserScheduling", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			var (
				setCalled      int64
				allowAutostart atomic.Bool
				allowAutostop  atomic.Bool
			)
			allowAutostart.Store(true)
			allowAutostop.Store(true)
			client := coderdtest.New(t, &coderdtest.Options{
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						atomic.AddInt64(&setCalled, 1)
						assert.Equal(t, allowAutostart.Load(), options.UserAutostartEnabled)
						assert.Equal(t, allowAutostop.Load(), options.UserAutostopEnabled)

						template.DefaultTTL = int64(options.DefaultTTL)
						template.AllowUserAutostart = options.UserAutostartEnabled
						template.AllowUserAutostop = options.UserAutostopEnabled
						return template, nil
					},
				},
			})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			})
			require.Equal(t, allowAutostart.Load(), template.AllowUserAutostart)
			require.Equal(t, allowAutostop.Load(), template.AllowUserAutostop)

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			allowAutostart.Store(false)
			allowAutostop.Store(false)
			got, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
				Name:                         template.Name,
				DisplayName:                  template.DisplayName,
				Description:                  template.Description,
				Icon:                         template.Icon,
				DefaultTTLMillis:             template.DefaultTTLMillis,
				AutostopRequirement:          &template.AutostopRequirement,
				AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
				AllowUserAutostart:           allowAutostart.Load(),
				AllowUserAutostop:            allowAutostop.Load(),
			})
			require.NoError(t, err)

			require.EqualValues(t, 2, atomic.LoadInt64(&setCalled))
			require.Equal(t, allowAutostart.Load(), got.AllowUserAutostart)
			require.Equal(t, allowAutostop.Load(), got.AllowUserAutostop)
		})

		t.Run("IgnoredUnlicensed", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			})

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			got, err := client.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
				Name:        template.Name,
				DisplayName: template.DisplayName,
				Description: template.Description,
				Icon:        template.Icon,
				// Increase the default TTL to avoid error "not modified".
				DefaultTTLMillis:             template.DefaultTTLMillis + 1,
				AutostopRequirement:          &template.AutostopRequirement,
				AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
				AllowUserAutostart:           false,
				AllowUserAutostop:            false,
			})
			require.NoError(t, err)
			require.True(t, got.AllowUserAutostart)
			require.True(t, got.AllowUserAutostop)
		})
	})

	t.Run("NotModified", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.Icon = "/icon/original-icon.png"
			ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.UpdateTemplateMeta{
			Name:                template.Name,
			Description:         template.Description,
			Icon:                template.Icon,
			DefaultTTLMillis:    template.DefaultTTLMillis,
			AutostopRequirement: nil,
			AllowUserAutostart:  template.AllowUserAutostart,
			AllowUserAutostop:   template.AllowUserAutostop,
		}
		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.ErrorContains(t, err, "not modified")
		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Equal(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, template.Description, updated.Description)
		assert.Equal(t, template.Icon, updated.Icon)
		assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.DefaultTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.UpdateTemplateMeta{
			DefaultTTLMillis: -int64(time.Hour),
		}
		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Contains(t, apiErr.Message, "Invalid request")
		require.Len(t, apiErr.Validations, 1)
		assert.Equal(t, apiErr.Validations[0].Field, "default_ttl_ms")

		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.WithinDuration(t, template.UpdatedAt, updated.UpdatedAt, time.Minute)
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, template.Description, updated.Description)
		assert.Equal(t, template.Icon, updated.Icon)
		assert.Equal(t, template.DefaultTTLMillis, updated.DefaultTTLMillis)
	})

	t.Run("RemoveIcon", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Icon = "/icon/code.png"
		})
		req := codersdk.UpdateTemplateMeta{
			Icon: "",
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)
		assert.Equal(t, updated.Icon, "")
	})

	t.Run("AutostopRequirement", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			var setCalled int64
			client := coderdtest.New(t, &coderdtest.Options{
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						if atomic.AddInt64(&setCalled, 1) == 2 {
							assert.EqualValues(t, 0b0110000, options.AutostopRequirement.DaysOfWeek)
							assert.EqualValues(t, 2, options.AutostopRequirement.Weeks)
						}

						err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
							ID:                            template.ID,
							UpdatedAt:                     dbtime.Now(),
							AllowUserAutostart:            options.UserAutostartEnabled,
							AllowUserAutostop:             options.UserAutostopEnabled,
							DefaultTTL:                    int64(options.DefaultTTL),
							UseMaxTtl:                     options.UseMaxTTL,
							MaxTTL:                        int64(options.MaxTTL),
							AutostopRequirementDaysOfWeek: int16(options.AutostopRequirement.DaysOfWeek),
							AutostopRequirementWeeks:      options.AutostopRequirement.Weeks,
							FailureTTL:                    int64(options.FailureTTL),
							TimeTilDormant:                int64(options.TimeTilDormant),
							TimeTilDormantAutoDelete:      int64(options.TimeTilDormantAutoDelete),
						})
						if !assert.NoError(t, err) {
							return database.Template{}, err
						}

						return db.GetTemplateByID(ctx, template.ID)
					},
				},
			})
			user := coderdtest.CreateFirstUser(t, client)

			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			require.EqualValues(t, 1, atomic.LoadInt64(&setCalled))
			require.Empty(t, template.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 1, template.AutostopRequirement.Weeks)
			req := codersdk.UpdateTemplateMeta{
				Name:                         template.Name,
				DisplayName:                  template.DisplayName,
				Description:                  template.Description,
				Icon:                         template.Icon,
				AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
				DefaultTTLMillis:             time.Hour.Milliseconds(),
				AutostopRequirement: &codersdk.TemplateAutostopRequirement{
					// wrong order
					DaysOfWeek: []string{"saturday", "friday"},
					Weeks:      2,
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
			require.NoError(t, err)
			require.EqualValues(t, 2, atomic.LoadInt64(&setCalled))
			require.Equal(t, []string{"friday", "saturday"}, updated.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 2, updated.AutostopRequirement.Weeks)

			template, err = client.Template(ctx, template.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"friday", "saturday"}, template.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 2, template.AutostopRequirement.Weeks)
			require.Empty(t, template.DeprecationMessage)
			require.False(t, template.Deprecated)
		})

		t.Run("Unset", func(t *testing.T) {
			t.Parallel()

			var setCalled int64
			client := coderdtest.New(t, &coderdtest.Options{
				TemplateScheduleStore: schedule.MockTemplateScheduleStore{
					SetFn: func(ctx context.Context, db database.Store, template database.Template, options schedule.TemplateScheduleOptions) (database.Template, error) {
						if atomic.AddInt64(&setCalled, 1) == 2 {
							assert.EqualValues(t, 0, options.AutostopRequirement.DaysOfWeek)
							assert.EqualValues(t, 1, options.AutostopRequirement.Weeks)
						}

						err := db.UpdateTemplateScheduleByID(ctx, database.UpdateTemplateScheduleByIDParams{
							ID:                            template.ID,
							UpdatedAt:                     dbtime.Now(),
							AllowUserAutostart:            options.UserAutostartEnabled,
							AllowUserAutostop:             options.UserAutostopEnabled,
							DefaultTTL:                    int64(options.DefaultTTL),
							UseMaxTtl:                     options.UseMaxTTL,
							MaxTTL:                        int64(options.MaxTTL),
							AutostopRequirementDaysOfWeek: int16(options.AutostopRequirement.DaysOfWeek),
							AutostopRequirementWeeks:      options.AutostopRequirement.Weeks,
							FailureTTL:                    int64(options.FailureTTL),
							TimeTilDormant:                int64(options.TimeTilDormant),
							TimeTilDormantAutoDelete:      int64(options.TimeTilDormantAutoDelete),
						})
						if !assert.NoError(t, err) {
							return database.Template{}, err
						}

						return db.GetTemplateByID(ctx, template.ID)
					},
				},
			})
			user := coderdtest.CreateFirstUser(t, client)

			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
				ctr.AutostopRequirement = &codersdk.TemplateAutostopRequirement{
					// wrong order
					DaysOfWeek: []string{"sunday", "saturday", "friday", "thursday", "wednesday", "tuesday", "monday"},
					Weeks:      2,
				}
			})
			require.EqualValues(t, 1, atomic.LoadInt64(&setCalled))
			require.Equal(t, []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}, template.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 2, template.AutostopRequirement.Weeks)
			req := codersdk.UpdateTemplateMeta{
				Name:                         template.Name,
				DisplayName:                  template.DisplayName,
				Description:                  template.Description,
				Icon:                         template.Icon,
				AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
				DefaultTTLMillis:             time.Hour.Milliseconds(),
				AutostopRequirement: &codersdk.TemplateAutostopRequirement{
					DaysOfWeek: []string{},
					Weeks:      0,
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
			require.NoError(t, err)
			require.EqualValues(t, 2, atomic.LoadInt64(&setCalled))
			require.Empty(t, updated.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 1, updated.AutostopRequirement.Weeks)

			template, err = client.Template(ctx, template.ID)
			require.NoError(t, err)
			require.Empty(t, template.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 1, template.AutostopRequirement.Weeks)
		})

		t.Run("EnterpriseOnly", func(t *testing.T) {
			t.Parallel()

			client := coderdtest.New(t, nil)
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
			template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
			require.Empty(t, template.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 1, template.AutostopRequirement.Weeks)
			req := codersdk.UpdateTemplateMeta{
				Name:                         template.Name,
				DisplayName:                  template.DisplayName,
				Description:                  template.Description,
				Icon:                         template.Icon,
				AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
				DefaultTTLMillis:             time.Hour.Milliseconds(),
				AutostopRequirement: &codersdk.TemplateAutostopRequirement{
					DaysOfWeek: []string{"monday"},
					Weeks:      2,
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
			require.NoError(t, err)
			require.Empty(t, updated.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 1, updated.AutostopRequirement.Weeks)

			template, err = client.Template(ctx, template.ID)
			require.NoError(t, err)
			require.Empty(t, template.AutostopRequirement.DaysOfWeek)
			require.EqualValues(t, 1, template.AutostopRequirement.Weeks)
			require.Empty(t, template.DeprecationMessage)
			require.False(t, template.Deprecated)
		})
	})

	t.Run("BothMaxTTLAndAutostopRequirement", func(t *testing.T) {
		t.Parallel()

		// Fake template schedule store is unneeded for this test since the
		// route fails before it is called.
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		req := codersdk.UpdateTemplateMeta{
			Name:                         template.Name,
			DisplayName:                  template.DisplayName,
			Description:                  template.Description,
			Icon:                         template.Icon,
			AllowUserCancelWorkspaceJobs: template.AllowUserCancelWorkspaceJobs,
			DefaultTTLMillis:             time.Hour.Milliseconds(),
			MaxTTLMillis:                 time.Hour.Milliseconds(),
			AutostopRequirement: &codersdk.TemplateAutostopRequirement{
				DaysOfWeek: []string{"monday"},
				Weeks:      2,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.Error(t, err)
		require.ErrorContains(t, err, "max_ttl_ms")
	})
}

func TestDeleteTemplate(t *testing.T) {
	t.Parallel()

	t.Run("NoWorkspaces", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.DeleteTemplate(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, auditor.AuditLogs(), 5)
		assert.Equal(t, database.AuditActionDelete, auditor.AuditLogs()[4].Action)
	})

	t.Run("Workspaces", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
		coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.DeleteTemplate(ctx, template.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})
}

func TestTemplateMetrics(t *testing.T) {
	t.Parallel()

	t.Skip("flaky test: https://github.com/coder/coder/issues/6481")

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon:    true,
		AgentStatsRefreshInterval:   time.Millisecond * 100,
		MetricsCacheRefreshInterval: time.Millisecond * 100,
	})

	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	require.Equal(t, -1, template.ActiveUserCount)
	require.Empty(t, template.BuildTimeStats[codersdk.WorkspaceTransitionStart])

	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	_ = agenttest.New(t, client.URL, authToken)
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	daus, err := client.TemplateDAUs(context.Background(), template.ID, codersdk.TimezoneOffsetHour(time.UTC))
	require.NoError(t, err)

	require.Equal(t, &codersdk.DAUsResponse{
		Entries: []codersdk.DAUEntry{},
	}, daus, "no DAUs when stats are empty")

	res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	assert.Zero(t, res.Workspaces[0].LastUsedAt)

	conn, err := client.DialWorkspaceAgent(ctx, resources[0].Agents[0].ID, &codersdk.DialWorkspaceAgentOptions{
		Logger: slogtest.Make(t, nil).Named("tailnet"),
	})
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	sshConn, err := conn.SSHClient(ctx)
	require.NoError(t, err)
	_ = sshConn.Close()

	wantDAUs := &codersdk.DAUsResponse{
		Entries: []codersdk.DAUEntry{
			{
				Date:   time.Now().UTC().Truncate(time.Hour * 24),
				Amount: 1,
			},
		},
	}
	require.Eventuallyf(t, func() bool {
		daus, err = client.TemplateDAUs(ctx, template.ID, codersdk.TimezoneOffsetHour(time.UTC))
		require.NoError(t, err)
		return len(daus.Entries) > 0
	},
		testutil.WaitShort, testutil.IntervalFast,
		"template daus never loaded",
	)
	gotDAUs, err := client.TemplateDAUs(ctx, template.ID, codersdk.TimezoneOffsetHour(time.UTC))
	require.NoError(t, err)
	require.Equal(t, gotDAUs, wantDAUs)

	template, err = client.Template(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, 1, template.ActiveUserCount)

	require.Eventuallyf(t, func() bool {
		template, err = client.Template(ctx, template.ID)
		require.NoError(t, err)
		startMs := template.BuildTimeStats[codersdk.WorkspaceTransitionStart].P50
		return startMs != nil && *startMs > 1
	},
		testutil.WaitShort, testutil.IntervalFast,
		"BuildTimeStats never loaded",
	)

	res, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	assert.WithinDuration(t,
		dbtime.Now(), res.Workspaces[0].LastUsedAt, time.Minute,
	)
}
