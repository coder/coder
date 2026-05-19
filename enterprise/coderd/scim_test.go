package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/enterprise/coderd/scim"
	"github.com/coder/coder/v2/testutil"
)

// scimUser is the JSON shape Coder accepts and emits for SCIM User
// resources. The struct is shared by tests for request bodies and
// response decoding; field tags match RFC 7643 Section 4.1.
type scimUser struct {
	Schemas    []string `json:"schemas,omitempty"`
	ID         string   `json:"id,omitempty"`
	ExternalID string   `json:"externalId,omitempty"`
	UserName   string   `json:"userName"`
	Name       struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
	Emails []struct {
		Primary bool   `json:"primary"`
		Value   string `json:"value" format:"email"`
		Type    string `json:"type,omitempty"`
		Display string `json:"display,omitempty"`
	} `json:"emails"`
	// Active is a ptr so we can distinguish "unset" from "false" in
	// requests; the legacy handler enforced presence and the new one
	// still does.
	Active *bool                  `json:"active,omitempty"`
	Meta   map[string]interface{} `json:"meta,omitempty"`
}

const (
	userSchemaURN  = "urn:ietf:params:scim:schemas:core:2.0:User"
	patchOpURN     = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	errorSchemaURN = "urn:ietf:params:scim:api:messages:2.0:Error"
)

//nolint:revive
func makeScimUser(t testing.TB) scimUser {
	rstr, err := cryptorand.String(10)
	require.NoError(t, err)

	u := scimUser{
		Schemas:  []string{userSchemaURN},
		UserName: rstr,
		Active:   ptr.Ref(true),
	}
	u.Name.GivenName = rstr
	u.Name.FamilyName = rstr
	u.Emails = append(u.Emails, struct {
		Primary bool   `json:"primary"`
		Value   string `json:"value" format:"email"`
		Type    string `json:"type,omitempty"`
		Display string `json:"display,omitempty"`
	}{Primary: true, Value: fmt.Sprintf("%s@coder.com", rstr)})
	return u
}

// scimPatchOp is the JSON shape of a single RFC 7644 PATCH operation.
type scimPatchOp struct {
	Op    string      `json:"op"`
	Path  string      `json:"path,omitempty"`
	Value interface{} `json:"value"`
}

// scimPatchBody is a SCIM 2.0 PatchOp request body.
type scimPatchBody struct {
	Schemas    []string      `json:"schemas"`
	Operations []scimPatchOp `json:"Operations"`
}

// replaceActive returns a PATCH body that toggles the `active` field.
func replaceActive(active bool) scimPatchBody {
	return scimPatchBody{
		Schemas: []string{patchOpURN},
		Operations: []scimPatchOp{
			{Op: "replace", Path: "active", Value: active},
		},
	}
}

func setScimAuth(key []byte) func(*http.Request) {
	return func(r *http.Request) {
		r.Header.Set("Authorization", string(key))
	}
}

func setScimAuthBearer(key []byte) func(*http.Request) {
	return func(r *http.Request) {
		// Do strange casing to ensure it's case-insensitive
		r.Header.Set("Authorization", "beAreR "+string(key))
	}
}

// decodeScimUser reads the response body into a scimUser, failing the
// test on JSON errors. We tolerate the response containing additional
// fields (meta, schemas, etc.) that scimUser also models.
func decodeScimUser(t testing.TB, body io.Reader) scimUser {
	t.Helper()
	var u scimUser
	require.NoError(t, json.NewDecoder(body).Decode(&u))
	return u
}

//nolint:gocritic // SCIM authenticates via a special header and bypasses internal RBAC.
func TestScim(t *testing.T) {
	t.Parallel()

	t.Run("postUser", func(t *testing.T) {
		t.Parallel()

		t.Run("disabled", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: []byte("hi"),
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 0,
					},
				},
			})

			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", struct{}{})
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusForbidden, res.StatusCode)
		})

		t.Run("noAuth", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: []byte("hi"),
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", struct{}{})
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			notifyEnq := &notificationstest.FakeEnqueuer{}
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:               mockAudit,
					NotificationsEnqueuer: notifyEnq,
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			// /ServiceProviderConfig is unauthenticated by spec; verify
			// it serves while SCIM is enabled.
			res, err := client.Request(ctx, http.MethodGet, "/scim/v2/ServiceProviderConfig", nil)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)

			sUser := makeScimUser(t)
			res, err = client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusCreated, res.StatusCode)

			// Audit log shape and marker fields.
			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			af := map[string]string{}
			require.NoError(t, json.Unmarshal([]byte(aLogs[0].AdditionalFields), &af))
			assert.Equal(t, scim.AuditAdditionalFields(), af)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			// User is now visible over the Coder API.
			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
			assert.Len(t, userRes.Users[0].OrganizationIDs, 1)

			// SkipNotifications = true.
			require.Empty(t, notifyEnq.Sent())
		})

		t.Run("OK_Bearer", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			notifyEnq := &notificationstest.FakeEnqueuer{}
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:               mockAudit,
					NotificationsEnqueuer: notifyEnq,
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuthBearer(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusCreated, res.StatusCode)

			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			af := map[string]string{}
			require.NoError(t, json.Unmarshal([]byte(aLogs[0].AdditionalFields), &af))
			assert.Equal(t, scim.AuditAdditionalFields(), af)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
			assert.Len(t, userRes.Users[0].OrganizationIDs, 1)
			require.Empty(t, notifyEnq.Sent())
		})

		t.Run("OKNoDefault", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			notifyEnq := &notificationstest.FakeEnqueuer{}
			dv := coderdtest.DeploymentValues(t)
			dv.OIDC.OrganizationAssignDefault = false
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:               mockAudit,
					NotificationsEnqueuer: notifyEnq,
					DeploymentValues:      dv,
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusCreated, res.StatusCode)

			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			af := map[string]string{}
			require.NoError(t, json.Unmarshal([]byte(aLogs[0].AdditionalFields), &af))
			assert.Equal(t, scim.AuditAdditionalFields(), af)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
			assert.Len(t, userRes.Users[0].OrganizationIDs, 0)
			require.Empty(t, notifyEnq.Sent())
		})

		t.Run("Duplicate", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: scimAPIKey,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			sUser := makeScimUser(t)
			// Pre-refactor behavior on duplicate POST was 200; with
			// elimity the framework always responds 201 on Create
			// success regardless of duplicate or new. We preserve the
			// "no 409 on duplicate" Okta-cloud quirk: the second and
			// third POST return the existing user as a fresh create.
			for i := 0; i < 3; i++ {
				res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
				require.NoError(t, err)
				_ = res.Body.Close()
				assert.Equal(t, http.StatusCreated, res.StatusCode)
			}

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
		})

		t.Run("Unsuspend", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: scimAPIKey,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusCreated, res.StatusCode)
			created := decodeScimUser(t, res.Body)
			require.NotEmpty(t, created.ID)

			// Suspend via PATCH.
			res, err = client.Request(ctx, http.MethodPatch, "/scim/v2/Users/"+created.ID, replaceActive(false), setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			// POST again with active=true; the create handler should
			// detect the suspended user and transition them back to
			// dormant.
			res, err = client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusCreated, res.StatusCode)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			assert.Equal(t, sUser.UserName, userRes.Users[0].Username)
			assert.Equal(t, codersdk.UserStatusDormant, userRes.Users[0].Status)
		})

		t.Run("DomainStrips", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: scimAPIKey,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			sUser := makeScimUser(t)
			sUser.UserName = sUser.UserName + "@coder.com"
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusCreated, res.StatusCode)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, sUser.Emails[0].Value, userRes.Users[0].Email)
			// Username should match what was given before the domain
			// suffix was appended.
			assert.Equal(t, sUser.Name.GivenName, userRes.Users[0].Username)
		})
	})

	t.Run("patchUser", func(t *testing.T) {
		t.Parallel()

		t.Run("disabled", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: []byte("hi"),
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 0,
					},
				},
			})

			res, err := client.Request(ctx, http.MethodPatch, "/scim/v2/Users/"+uuid.NewString(), replaceActive(false))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusForbidden, res.StatusCode)
		})

		t.Run("noAuth", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: []byte("hi"),
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			res, err := client.Request(ctx, http.MethodPatch, "/scim/v2/Users/"+uuid.NewString(), replaceActive(false))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options:      &coderdtest.Options{Auditor: mockAudit},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			require.Equal(t, http.StatusCreated, res.StatusCode)
			created := decodeScimUser(t, res.Body)
			_ = res.Body.Close()
			require.NotEmpty(t, created.ID)
			mockAudit.ResetLogs()

			res, err = client.Request(ctx, http.MethodPatch, "/scim/v2/Users/"+created.ID, replaceActive(false), setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			assert.Equal(t, database.AuditActionWrite, aLogs[0].Action)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, codersdk.UserStatusSuspended, userRes.Users[0].Status)
		})

		// Create a user via SCIM (dormant), log them in (active),
		// PATCH them again with no change, and confirm they remain
		// active with no extra audit log entry.
		t.Run("ActiveIsActive", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")

			mockAudit := audit.NewMock()
			fake := oidctest.NewFakeIDP(t, oidctest.WithServing())
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:    mockAudit,
					OIDCConfig: fake.OIDCConfig(t, []string{}),
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			require.Equal(t, http.StatusCreated, res.StatusCode)
			created := decodeScimUser(t, res.Body)
			_ = res.Body.Close()
			require.NotEmpty(t, created.ID)

			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			scimCoderUser, err := client.User(ctx, sUser.UserName)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusDormant, scimCoderUser.Status, "user starts as dormant")

			//nolint:bodyclose
			scimUserClient, _ := fake.Login(t, client, jwt.MapClaims{
				"email": sUser.Emails[0].Value,
				"sub":   uuid.NewString(),
			})
			scimCoderUser, err = scimUserClient.User(ctx, codersdk.Me)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusActive, scimCoderUser.Status, "user should now be active")

			mockAudit.ResetLogs()
			res, err = client.Request(ctx, http.MethodPatch, "/scim/v2/Users/"+created.ID, replaceActive(true), setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			// No audit log expected since no diff.
			aLogs = mockAudit.AuditLogs()
			require.Len(t, aLogs, 0)

			scimCoderUser, err = client.User(ctx, sUser.UserName)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusActive, scimCoderUser.Status, "user is still active")
		})
	})

	t.Run("putUser", func(t *testing.T) {
		t.Parallel()

		t.Run("disabled", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: []byte("hi"),
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 0,
					},
				},
			})

			res, err := client.Request(ctx, http.MethodPut, "/scim/v2/Users/"+uuid.NewString(), makeScimUser(t))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusForbidden, res.StatusCode)
		})

		t.Run("noAuth", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				SCIMAPIKey: []byte("hi"),
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM: 1,
					},
				},
			})

			res, err := client.Request(ctx, http.MethodPut, "/scim/v2/Users/"+uuid.NewString(), makeScimUser(t))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		})

		t.Run("MissingActiveField", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options:      &coderdtest.Options{Auditor: mockAudit},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			require.Equal(t, http.StatusCreated, res.StatusCode)
			created := decodeScimUser(t, res.Body)
			_ = res.Body.Close()
			require.NotEmpty(t, created.ID)
			mockAudit.ResetLogs()

			noActive := sUser
			noActive.Active = nil
			res, err = client.Request(ctx, http.MethodPut, "/scim/v2/Users/"+created.ID, noActive, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusBadRequest, res.StatusCode)

			data, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Contains(t, string(data), "active field is required")
		})

		t.Run("ImmutabilityViolation", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options:      &coderdtest.Options{Auditor: mockAudit},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			require.Equal(t, http.StatusCreated, res.StatusCode)
			created := decodeScimUser(t, res.Body)
			_ = res.Body.Close()
			require.NotEmpty(t, created.ID)
			mockAudit.ResetLogs()

			changed := sUser
			changed.UserName = sUser.UserName + "changed"
			res, err = client.Request(ctx, http.MethodPut, "/scim/v2/Users/"+created.ID, changed, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Equal(t, http.StatusBadRequest, res.StatusCode)

			data, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Contains(t, string(data), "mutability")
		})

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options:      &coderdtest.Options{Auditor: mockAudit},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			require.Equal(t, http.StatusCreated, res.StatusCode)
			created := decodeScimUser(t, res.Body)
			_ = res.Body.Close()
			require.NotEmpty(t, created.ID)
			mockAudit.ResetLogs()

			suspend := sUser
			suspend.Active = ptr.Ref(false)
			res, err = client.Request(ctx, http.MethodPut, "/scim/v2/Users/"+created.ID, suspend, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			assert.Equal(t, database.AuditActionWrite, aLogs[0].Action)

			userRes, err := client.Users(ctx, codersdk.UsersRequest{Search: sUser.Emails[0].Value})
			require.NoError(t, err)
			require.Len(t, userRes.Users, 1)
			assert.Equal(t, codersdk.UserStatusSuspended, userRes.Users[0].Status)
		})

		// Same shape as patchUser/ActiveIsActive: ensure PUT with no
		// change to an active user does not regress them to dormant
		// and does not produce a spurious audit log.
		t.Run("ActiveIsActive", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			scimAPIKey := []byte("hi")
			mockAudit := audit.NewMock()
			fake := oidctest.NewFakeIDP(t, oidctest.WithServing())
			client, _ := coderdenttest.New(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					Auditor:    mockAudit,
					OIDCConfig: fake.OIDCConfig(t, []string{}),
				},
				SCIMAPIKey:   scimAPIKey,
				AuditLogging: true,
				LicenseOptions: &coderdenttest.LicenseOptions{
					AccountID: "coolin",
					Features: license.Features{
						codersdk.FeatureSCIM:     1,
						codersdk.FeatureAuditLog: 1,
					},
				},
			})
			mockAudit.ResetLogs()

			sUser := makeScimUser(t)
			res, err := client.Request(ctx, http.MethodPost, "/scim/v2/Users", sUser, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			require.Equal(t, http.StatusCreated, res.StatusCode)
			created := decodeScimUser(t, res.Body)
			_ = res.Body.Close()
			require.NotEmpty(t, created.ID)

			aLogs := mockAudit.AuditLogs()
			require.Len(t, aLogs, 1)
			assert.Equal(t, database.AuditActionCreate, aLogs[0].Action)

			scimCoderUser, err := client.User(ctx, sUser.UserName)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusDormant, scimCoderUser.Status, "user starts as dormant")

			//nolint:bodyclose
			scimUserClient, _ := fake.Login(t, client, jwt.MapClaims{
				"email": sUser.Emails[0].Value,
				"sub":   uuid.NewString(),
			})
			scimCoderUser, err = scimUserClient.User(ctx, codersdk.Me)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusActive, scimCoderUser.Status, "user should now be active")

			mockAudit.ResetLogs()
			same := sUser
			same.ID = created.ID
			res, err = client.Request(ctx, http.MethodPut, "/scim/v2/Users/"+created.ID, same, setScimAuth(scimAPIKey))
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, http.StatusOK, res.StatusCode)

			aLogs = mockAudit.AuditLogs()
			require.Len(t, aLogs, 0)

			scimCoderUser, err = client.User(ctx, sUser.UserName)
			require.NoError(t, err)
			require.Equal(t, codersdk.UserStatusActive, scimCoderUser.Status, "user is still active")
		})
	})
}
