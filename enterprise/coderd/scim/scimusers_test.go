package scim

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/elimity-com/scim"
	scimErrors "github.com/elimity-com/scim/errors"
	"github.com/google/uuid"
	filter "github.com/scim2/filter-parser/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

// setupSCIM creates a ResourceUser backed by a real database for testing.
// The returned mock auditor can be inspected for emitted audit logs.
func setupSCIM(t *testing.T) (*ResourceUser, database.Store, *audit.MockAuditor) {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	mockAudit := audit.NewMock()
	auditorPtr := atomic.Pointer[audit.Auditor]{}
	var a audit.Auditor = mockAudit
	auditorPtr.Store(&a)

	ru := &ResourceUser{
		store: db,
		opts: &Options{
			DB:      db,
			Auditor: &auditorPtr,
			Logger:  slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
		},
	}
	return ru, db, mockAudit
}

// scimRequest builds an *http.Request with scim provisioner context,
// simulating the auth context that the SCIM middleware normally sets.
func scimRequest(t *testing.T) *http.Request {
	t.Helper()
	ctx := dbauthz.AsSCIMProvisioner(context.Background())
	return httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
}

// seedUser creates a user in the database for testing.
func seedUser(t *testing.T, db database.Store, opts database.User) database.User {
	t.Helper()
	return dbgen.User(t, db, opts)
}

// setupSCIMMock creates a ResourceUser backed by a gomock store for tests
// that only need to verify call patterns (e.g. audit emission) without
// real SQL.
func setupSCIMMock(t *testing.T) (*ResourceUser, *dbmock.MockStore, *audit.MockAuditor) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockStore := dbmock.NewMockStore(ctrl)
	mockAudit := audit.NewMock()
	auditorPtr := atomic.Pointer[audit.Auditor]{}
	var a audit.Auditor = mockAudit
	auditorPtr.Store(&a)

	ru := &ResourceUser{
		store: mockStore,
		opts: &Options{
			DB:      mockStore,
			Auditor: &auditorPtr,
			Logger:  slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
		},
	}
	return ru, mockStore, mockAudit
}

// --- Pure function tests (no DB) ---

func TestScimUserStatus(t *testing.T) {
	t.Parallel()

	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name     string
		status   database.UserStatus
		active   *bool
		expected database.UserStatus
	}{
		{"active+true=active", database.UserStatusActive, boolPtr(true), database.UserStatusActive},
		{"active+false=suspended", database.UserStatusActive, boolPtr(false), database.UserStatusSuspended},
		{"suspended+true=dormant", database.UserStatusSuspended, boolPtr(true), database.UserStatusDormant},
		{"suspended+false=suspended", database.UserStatusSuspended, boolPtr(false), database.UserStatusSuspended},
		{"dormant+true=dormant", database.UserStatusDormant, boolPtr(true), database.UserStatusDormant},
		{"dormant+false=suspended", database.UserStatusDormant, boolPtr(false), database.UserStatusSuspended},
		{"active+nil=active", database.UserStatusActive, nil, database.UserStatusActive},
		{"suspended+nil=suspended", database.UserStatusSuspended, nil, database.UserStatusSuspended},
		{"dormant+nil=dormant", database.UserStatusDormant, nil, database.UserStatusDormant},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			user := database.User{Status: tt.status}
			got := scimUserStatus(user, tt.active)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestPrimaryEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		attrs    scim.ResourceAttributes
		expected string
	}{
		{
			name: "primary email",
			attrs: scim.ResourceAttributes{
				"emails": []interface{}{
					map[string]interface{}{"value": "a@b.com", "primary": true},
				},
			},
			expected: "a@b.com",
		},
		{
			name: "fallback to first when no primary",
			attrs: scim.ResourceAttributes{
				"emails": []interface{}{
					map[string]interface{}{"value": "first@b.com"},
				},
			},
			expected: "first@b.com",
		},
		{
			name: "picks primary over first",
			attrs: scim.ResourceAttributes{
				"emails": []interface{}{
					map[string]interface{}{"value": "first@b.com"},
					map[string]interface{}{"value": "primary@b.com", "primary": true},
				},
			},
			expected: "primary@b.com",
		},
		{
			name: "polluted",
			attrs: scim.ResourceAttributes{
				"emails": []interface{}{
					// Try and cause a panic
					"not-a-map",
					true,
					7,
					map[int]interface{}{
						1: "bad",
					},
					map[string]interface{}{
						"value": 123, // value is not a string
					},
					map[string]interface{}{},
					map[string]interface{}{"value": "first@b.com"},
					map[string]interface{}{"value": "primary@b.com", "primary": true},
				},
			},
			expected: "primary@b.com",
		},
		{
			name:     "no emails key",
			attrs:    scim.ResourceAttributes{},
			expected: "",
		},
		{
			name:     "empty emails",
			attrs:    scim.ResourceAttributes{"emails": []interface{}{}},
			expected: "",
		},
		{
			name:     "wrong type",
			attrs:    scim.ResourceAttributes{"emails": "not-a-list"},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := primaryEmail(tt.attrs)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestBooleanValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		want    bool
		wantErr bool
	}{
		{"bool true", true, true, false},
		{"bool false", false, false, false},
		{"string true", "true", true, false},
		{"string false", "false", false, false},
		{"string True", "True", true, false},
		{"int", 42, false, true},
		{"nil", nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := BooleanValue(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// --- Handler tests (with DB) ---

func TestResourceUser_Lifecycle(t *testing.T) {
	t.Parallel()

	ru, db, _ := setupSCIM(t)

	// Seed an active user.
	user := seedUser(t, db, database.User{
		Status:    database.UserStatusActive,
		LoginType: database.LoginTypeOIDC,
	})

	r := scimRequest(t)

	// Step 1: Get the user. Verify fields match.
	res, err := ru.Get(r, user.ID.String())
	require.NoError(t, err)
	assert.Equal(t, user.ID.String(), res.ID)
	assert.Equal(t, user.Username, res.Attributes["userName"])
	assert.Equal(t, true, res.Attributes["active"])

	// Step 2: Replace with active=false → suspended.
	res, err = ru.Replace(r, user.ID.String(), scim.ResourceAttributes{
		"userName": user.Username,
		"active":   false,
	})
	require.NoError(t, err)
	assert.Equal(t, false, res.Attributes["active"])

	// Step 3: Get → confirm inactive.
	res, err = ru.Get(r, user.ID.String())
	require.NoError(t, err)
	assert.Equal(t, false, res.Attributes["active"])

	// Step 4: Patch active=true → dormant (shown as active in SCIM).
	res, err = ru.Patch(r, user.ID.String(), []scim.PatchOperation{
		{Op: "replace", Path: mustPath("active"), Value: true},
	})
	require.NoError(t, err)
	assert.Equal(t, true, res.Attributes["active"])

	// Step 5: Get → confirm active again.
	res, err = ru.Get(r, user.ID.String())
	require.NoError(t, err)
	assert.Equal(t, true, res.Attributes["active"])

	// Step 6: Delete → suspended.
	err = ru.Delete(r, user.ID.String())
	require.NoError(t, err)

	// Step 7: Get → confirm inactive after delete.
	res, err = ru.Get(r, user.ID.String())
	require.NoError(t, err)
	assert.Equal(t, false, res.Attributes["active"])
}

func TestResourceUser_GetAll(t *testing.T) {
	t.Parallel()

	ru, db, _ := setupSCIM(t)

	// Seed 3 users.
	for i := 0; i < 3; i++ {
		seedUser(t, db, database.User{
			LoginType: database.LoginTypeOIDC,
		})
	}

	r := scimRequest(t)

	// Get all with large count.
	page, err := ru.GetAll(r, scim.ListRequestParams{Count: 100, StartIndex: 1})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, page.TotalResults, 3)
	assert.GreaterOrEqual(t, len(page.Resources), 3)

	// Paginate: startIndex=2, count=1.
	page, err = ru.GetAll(r, scim.ListRequestParams{Count: 1, StartIndex: 2})
	require.NoError(t, err)
	assert.Len(t, page.Resources, 1)
	assert.GreaterOrEqual(t, page.TotalResults, 3)
}

func TestResourceUser_Errors(t *testing.T) {
	t.Parallel()

	ru, _, _ := setupSCIM(t)
	r := scimRequest(t)
	missingUUID := uuid.New().String()

	tests := []struct {
		name       string
		run        func() error
		wantStatus int
	}{
		{
			name:       "Get/non-UUID",
			run:        func() error { _, err := ru.Get(r, "not-a-uuid"); return err },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Get/missing",
			run:        func() error { _, err := ru.Get(r, missingUUID); return err },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Replace/non-UUID",
			run:        func() error { _, err := ru.Replace(r, "bad", scim.ResourceAttributes{}); return err },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Replace/missing",
			run:        func() error { _, err := ru.Replace(r, missingUUID, scim.ResourceAttributes{}); return err },
			wantStatus: http.StatusNotFound,
		},
		{
			name: "Replace/immutable-userName",
			run: func() error {
				// Need a real user for this test.
				user := seedUser(t, ru.store, database.User{LoginType: database.LoginTypeOIDC})
				_, err := ru.Replace(r, user.ID.String(), scim.ResourceAttributes{
					"userName": "different-name",
				})
				return err
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Patch/non-UUID",
			run:        func() error { _, err := ru.Patch(r, "bad", nil); return err },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Patch/missing",
			run:        func() error { _, err := ru.Patch(r, missingUUID, nil); return err },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Delete/non-UUID",
			run:        func() error { return ru.Delete(r, "bad") },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Delete/missing",
			run:        func() error { return ru.Delete(r, missingUUID) },
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			require.Error(t, err)
			var scimErr scimErrors.ScimError
			require.ErrorAs(t, err, &scimErr)
			assert.Equal(t, tt.wantStatus, scimErr.Status)
		})
	}
}

func TestResourceUser_AuditLogs(t *testing.T) {
	t.Parallel()

	// These tests use dbmock instead of a real database because they only
	// verify audit emission logic (does an audit log fire when status
	// changes?), not SQL correctness. The handlers call just GetUserByID
	// and UpdateUserStatus, both trivially mockable.

	makeUser := func(status database.UserStatus) (database.User, database.User) {
		id := uuid.New()
		user := database.User{
			ID:        id,
			Username:  "testuser",
			Status:    status,
			LoginType: database.LoginTypeOIDC,
		}
		suspended := user
		suspended.Status = database.UserStatusSuspended
		return user, suspended
	}

	t.Run("Replace/status-change-emits-audit", func(t *testing.T) {
		t.Parallel()
		ru, mockStore, mockAudit := setupSCIMMock(t)
		activeUser, suspendedUser := makeUser(database.UserStatusActive)

		mockStore.EXPECT().GetUserByID(gomock.Any(), activeUser.ID).Return(activeUser, nil)
		mockStore.EXPECT().UpdateUserStatus(gomock.Any(), gomock.Any()).Return(suspendedUser, nil)

		_, err := ru.Replace(scimRequest(t), activeUser.ID.String(), scim.ResourceAttributes{
			"userName": activeUser.Username,
			"active":   false,
		})
		require.NoError(t, err)
		assert.Len(t, mockAudit.AuditLogs(), 1)
	})

	t.Run("Replace/no-change-skips-audit", func(t *testing.T) {
		t.Parallel()
		ru, mockStore, mockAudit := setupSCIMMock(t)
		activeUser, _ := makeUser(database.UserStatusActive)

		mockStore.EXPECT().GetUserByID(gomock.Any(), activeUser.ID).Return(activeUser, nil)
		// No UpdateUserStatus expected: active=true on an already active user is a no-op.

		_, err := ru.Replace(scimRequest(t), activeUser.ID.String(), scim.ResourceAttributes{
			"userName": activeUser.Username,
			"active":   true,
		})
		require.NoError(t, err)
		assert.Empty(t, mockAudit.AuditLogs())
	})

	t.Run("Delete/active-user-emits-audit", func(t *testing.T) {
		t.Parallel()
		ru, mockStore, mockAudit := setupSCIMMock(t)
		activeUser, suspendedUser := makeUser(database.UserStatusActive)

		mockStore.EXPECT().GetUserByID(gomock.Any(), activeUser.ID).Return(activeUser, nil)
		mockStore.EXPECT().UpdateUserStatus(gomock.Any(), gomock.Any()).Return(suspendedUser, nil)

		err := ru.Delete(scimRequest(t), activeUser.ID.String())
		require.NoError(t, err)
		assert.Len(t, mockAudit.AuditLogs(), 1)
	})

	t.Run("Delete/suspended-user-skips-audit", func(t *testing.T) {
		t.Parallel()
		ru, mockStore, mockAudit := setupSCIMMock(t)
		_, suspendedUser := makeUser(database.UserStatusSuspended)

		mockStore.EXPECT().GetUserByID(gomock.Any(), suspendedUser.ID).Return(suspendedUser, nil)
		// No UpdateUserStatus expected: already suspended.

		err := ru.Delete(scimRequest(t), suspendedUser.ID.String())
		require.NoError(t, err)
		assert.Empty(t, mockAudit.AuditLogs())
	})
}

// mustPath parses a SCIM attribute path string into a *filter.Path
// for use in PatchOperation test data.
func mustPath(attr string) *filter.Path {
	p, err := filter.ParsePath([]byte(attr))
	if err != nil {
		panic(fmt.Sprintf("mustPath(%q): %v", attr, err))
	}
	return &p
}
