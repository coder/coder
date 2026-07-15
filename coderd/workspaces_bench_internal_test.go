package coderd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
)

// BenchmarkWorkspacesHandler measures the authorization cost of the real
// GET /api/v2/workspaces HTTP handler (api.workspaces) for a user that belongs
// to a growing number of organizations.
//
// It exercises the full handler (api.workspaces) rather than
// dbauthz.GetWorkspaces so that the whole authorization path a request takes,
// including the handler-level HTTPAuth.AuthorizeSQLFilter call, is measured.
// The database is mocked so the benchmark isolates the authorization work.
//
// Partial evaluation cost is driven by the subject, not the object type: OPA
// expands the policy against the subject's N org-scoped roles, so it scales
// with org count (see #21890).
func BenchmarkWorkspacesHandler(b *testing.B) {
	orgCounts := []int{1, 10, 50, 100, 200}

	for _, n := range orgCounts {
		b.Run(fmt.Sprintf("orgs=%d", n), func(b *testing.B) {
			ctrl := gomock.NewController(b)
			mockDB := dbmock.NewMockStore(ctrl)
			mockDB.EXPECT().Wrappers().Return([]string{}).AnyTimes()

			userID := uuid.New()

			// The subject is built by the real ExtractAPIKey middleware path
			// (below), which resolves the user's roles via
			// GetAuthorizationUserRoles -> rolestore.Expand -> CustomRoles. Set
			// up the two lookups that path makes so the subject matches what a
			// real request produces (fully expanded roles, cached AST value).
			roleNames := make([]string, 0, n+1)
			roleNames = append(roleNames, rbac.RoleMember().String())
			for range n {
				orgRole := rbac.RoleIdentifier{Name: rbac.RoleOrgMember(), OrganizationID: uuid.New()}
				roleNames = append(roleNames, orgRole.String())
			}
			mockDB.EXPECT().
				GetAuthorizationUserRoles(gomock.Any(), userID).
				Return(database.GetAuthorizationUserRolesRow{
					ID:       userID,
					Username: "bench",
					Status:   database.UserStatusActive,
					Email:    "bench@coder.com",
					Roles:    roleNames,
				}, nil).
				AnyTimes()

			// CustomRoles resolves the organization-member system role for each
			// org the user belongs to, the same way the real query does with
			// IncludeSystemRoles.
			memberPerms := rbac.OrgMemberPermissions(rbac.OrgSettings{})
			mockDB.EXPECT().
				CustomRoles(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, arg database.CustomRolesParams) ([]database.CustomRole, error) {
					out := make([]database.CustomRole, 0, len(arg.LookupRoles))
					for _, pair := range arg.LookupRoles {
						out = append(out, database.CustomRole{
							Name:              pair.Name,
							OrganizationID:    uuid.NullUUID{UUID: pair.OrganizationID, Valid: pair.OrganizationID != uuid.Nil},
							IsSystem:          true,
							OrgPermissions:    rolestore.ConvertPermissionsToDB(memberPerms.Org),
							MemberPermissions: rolestore.ConvertPermissionsToDB(memberPerms.Member),
						})
					}
					return out, nil
				}).
				AnyTimes()

			// Generate a real token so ExtractAPIKey's hash validation passes,
			// and answer the key lookups it makes. These run once (during the
			// capture below), not in the measured loop.
			insertParams, token, err := apikey.Generate(apikey.CreateParams{
				UserID:          userID,
				LoginType:       database.LoginTypeToken,
				DefaultLifetime: time.Hour,
			})
			if err != nil {
				b.Fatal(err)
			}
			dbKey := database.APIKey{
				ID:              insertParams.ID,
				HashedSecret:    insertParams.HashedSecret,
				UserID:          userID,
				ExpiresAt:       insertParams.ExpiresAt,
				LastUsed:        insertParams.LastUsed,
				LoginType:       database.LoginTypeToken,
				LifetimeSeconds: insertParams.LifetimeSeconds,
				Scopes:          insertParams.Scopes,
				AllowList:       insertParams.AllowList,
			}
			mockDB.EXPECT().GetAPIKeyByID(gomock.Any(), insertParams.ID).Return(dbKey, nil).AnyTimes()
			mockDB.EXPECT().UpdateAPIKeyByID(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			mockDB.EXPECT().UpdateUserLastSeenAt(gomock.Any(), gomock.Any()).Return(database.User{ID: userID}, nil).AnyTimes()

			// Return a single technical summary row so the handler takes its
			// early-return path (workspaces.go: len(workspaceRows) == 1),
			// skipping enrichment. The measured cost is the per-request Prepare,
			// not row conversion. Mirror sqlQuerier.GetAuthorizedWorkspaces by
			// compiling the prepared authorizer to SQL, so CompileToSQL is
			// captured too.
			summaryRow := []database.GetWorkspacesRow{{}}
			mockDB.EXPECT().
				GetAuthorizedWorkspaces(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, _ database.GetWorkspacesParams, prepared rbac.PreparedAuthorized) ([]database.GetWorkspacesRow, error) {
					if _, err := prepared.CompileToSQL(ctx, rbac.ConfigWorkspaces()); err != nil {
						return nil, err
					}
					return summaryRow, nil
				}).
				AnyTimes()

			// Use a non-caching authorizer so this measures the cold cost.
			authorizer := rbac.NewAuthorizer(prometheus.NewRegistry())
			logger := slog.Make()

			acs := &atomic.Pointer[dbauthz.AccessControlStore]{}
			var tacs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
			acs.Store(&tacs)
			authzDB := dbauthz.New(mockDB, authorizer, logger, acs)

			api := &API{
				Options: &Options{
					Database:                       authzDB,
					Logger:                         logger,
					Authorizer:                     authorizer,
					AgentInactiveDisconnectTimeout: time.Minute,
				},
				// HTTPAuth backs the handler's AuthorizeSQLFilter call.
				HTTPAuth: &HTTPAuthorizer{Authorizer: authorizer, Logger: logger},
			}

			// Capture an authenticated request context once by running the real
			// ExtractAPIKey middleware. The captured context carries both the
			// apiKey value (read by httpmw.APIKey in the handler) and the
			// dbauthz actor (read by AuthorizeSQLFilter), so per-iteration cost
			// excludes subject construction and isolates the handler work.
			authedCtx := captureAuthedContext(b, mockDB, logger, token)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/api/v2/workspaces", nil).WithContext(authedCtx)
				api.workspaces(rec, req)
				if rec.Code != http.StatusOK {
					b.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
				}
			}
		})
	}
}

// captureAuthedContext runs the real ExtractAPIKey middleware a single time and
// returns the resulting request context, which carries the apiKey value and the
// dbauthz actor. There is no exported setter for the (unexported) apiKey context
// key, so running the middleware is the supported way to build a valid
// authenticated context.
func captureAuthedContext(b *testing.B, db database.Store, logger slog.Logger, token string) context.Context {
	b.Helper()

	var captured context.Context
	mw := httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{
		DB:     db,
		Logger: logger,
		// Bypass header/cookie parsing; return the generated token directly.
		SessionTokenFunc: func(*http.Request) string { return token },
	})
	handler := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r.Context()
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/workspaces", nil)
	handler.ServeHTTP(rec, req)
	if captured == nil {
		b.Fatalf("failed to authenticate benchmark request: status %d: %s", rec.Code, rec.Body.String())
	}
	return captured
}
