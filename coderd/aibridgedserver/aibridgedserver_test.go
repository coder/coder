package aibridgedserver_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	protobufproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"storj.io/drpc"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogjson"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/aibridgedserver"
	agplaiseats "github.com/coder/coder/v2/coderd/aiseats"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/externalauth"
	codermcp "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/notificationstest"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

var requiredExperiments = []codersdk.Experiment{
	codersdk.ExperimentMCPServerHTTP, codersdk.ExperimentOAuth2,
}

// TestAuthorization validates the authorization logic.
// No other tests are explicitly defined in this package because aibridgedserver is
// tested via integration tests in the aibridged package (see aibridged/aibridged_integration_test.go).
func TestAuthorization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// Key will be set to the same key passed to mocksFn if unset.
		key string
		// mocksFn is called with a valid API key and user. If the test needs
		// invalid values, it should just mutate them directly.
		mocksFn     func(db *dbmock.MockStore, apiKey database.APIKey, user database.User)
		expectedErr error
	}{
		{
			name:        "invalid key format",
			key:         "foo",
			expectedErr: aibridgedserver.ErrInvalidKey,
		},
		{
			name:        "unknown key",
			expectedErr: aibridgedserver.ErrUnknownKey,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(database.APIKey{}, sql.ErrNoRows)
			},
		},
		{
			name:        "expired",
			expectedErr: aibridgedserver.ErrExpired,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				apiKey.ExpiresAt = dbtime.Now().Add(-time.Hour)
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
			},
		},
		{
			name:        "invalid key secret",
			expectedErr: aibridgedserver.ErrInvalidKey,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				apiKey.HashedSecret = []byte("differentsecret")
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
			},
		},
		{
			name:        "unknown user",
			expectedErr: aibridgedserver.ErrUnknownUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(database.User{}, sql.ErrNoRows)
			},
		},
		{
			name:        "deleted user",
			expectedErr: aibridgedserver.ErrDeletedUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				user.Deleted = true
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
		{
			name:        "suspended user",
			expectedErr: aibridgedserver.ErrInactiveUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				user.Status = database.UserStatusSuspended
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
		{
			name:        "dormant user",
			expectedErr: aibridgedserver.ErrInactiveUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				user.Status = database.UserStatusDormant
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
		{
			name:        "system user",
			expectedErr: aibridgedserver.ErrSystemUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				user.IsSystem = true
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
		{
			name: "valid",
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := testutil.Logger(t)

			// Make a fake user and an API key for the mock calls.
			now := dbtime.Now()
			user := database.User{
				ID:         uuid.New(),
				Email:      "test@coder.com",
				Username:   "test",
				Name:       "Test User",
				CreatedAt:  now,
				UpdatedAt:  now,
				RBACRoles:  []string{},
				LoginType:  database.LoginTypePassword,
				Status:     database.UserStatusActive,
				LastSeenAt: now,
			}

			keyID, _ := cryptorand.String(10)
			keySecret, keySecretHashed, _ := apikey.GenerateSecret(22)
			token := fmt.Sprintf("%s-%s", keyID, keySecret)
			apiKey := database.APIKey{
				ID:              keyID,
				LifetimeSeconds: 86400, // default in db
				HashedSecret:    keySecretHashed,
				IPAddress: pqtype.Inet{
					IPNet: net.IPNet{
						IP:   net.IPv4(127, 0, 0, 1),
						Mask: net.IPv4Mask(255, 255, 255, 255),
					},
					Valid: true,
				},
				UserID:    user.ID,
				LastUsed:  now,
				ExpiresAt: now.Add(time.Hour),
				CreatedAt: now,
				UpdatedAt: now,
				LoginType: database.LoginTypePassword,
				Scopes:    []database.APIKeyScope{database.ApiKeyScopeCoderAll},
				TokenName: "",
			}
			if tc.key == "" {
				tc.key = token
			}

			// Define any case-specific mocks.
			if tc.mocksFn != nil {
				tc.mocksFn(db, apiKey, user)
			}

			srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{
				Store:         db,
				AISeatTracker: agplaiseats.Noop{},
				AccessURL:     "/",
				GatewayCfg:    codersdk.AIBridgeConfig{},
				Experiments:   requiredExperiments,
				Logger:        logger,
				Clock:         quartz.NewReal(),
			})
			require.NoError(t, err)
			require.NotNil(t, srv)

			resp, err := srv.IsAuthorized(t.Context(), &proto.IsAuthorizedRequest{Key: tc.key})
			if tc.expectedErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				expected := proto.IsAuthorizedResponse{
					OwnerId:  user.ID.String(),
					ApiKeyId: keyID,
					Username: user.Username,
				}
				require.NoError(t, err)
				require.Equal(t, &expected, resp)
			}
		})
	}
}

// When IsAuthorizedRequest carries KeyId instead of Key, the server skips
// the secret check and validates only that the key exists, is unexpired, and
// belongs to an active, non-deleted, non-system user. This is the path used by
// in-process delegated callers (e.g., chatd) that hold only the key ID.
func TestAuthorization_Delegated(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		mocksFn     func(db *dbmock.MockStore, apiKey database.APIKey, user database.User)
		bothFields  bool
		expectedErr error
	}{
		{
			name: "valid",
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
		{
			name:        "unknown key",
			expectedErr: aibridgedserver.ErrUnknownKey,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, _ database.User) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(database.APIKey{}, sql.ErrNoRows)
			},
		},
		{
			name:        "expired",
			expectedErr: aibridgedserver.ErrExpired,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, _ database.User) {
				apiKey.ExpiresAt = dbtime.Now().Add(-time.Hour)
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
			},
		},
		{
			// Sending both Key and KeyId is an API misuse and must be
			// rejected to avoid ambiguity about which path was taken.
			name:        "both fields set",
			bothFields:  true,
			expectedErr: aibridgedserver.ErrAmbiguousAuth,
		},
		{
			// A bogus secret has no effect on the delegated path because
			// the secret is never checked. This is the load-bearing
			// security property: trust is established out-of-band, not in
			// this RPC.
			name: "secret hash mismatch is ignored",
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				apiKey.HashedSecret = []byte("not-the-real-hash")
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
		{
			// The delegated path must still reject keys whose owner has
			// been deleted; trust at the transport boundary does not
			// extend to bypassing user-status checks.
			name:        "deleted user",
			expectedErr: aibridgedserver.ErrDeletedUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				user.Deleted = true
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
		{
			// The delegated path must reject inactive users; transport
			// trust does not override account suspension.
			name:        "suspended user",
			expectedErr: aibridgedserver.ErrInactiveUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				user.Status = database.UserStatusSuspended
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
		{
			// Dormant users are inactive unless they are explicitly
			// reactivated through the HTTP middleware path.
			name:        "dormant user",
			expectedErr: aibridgedserver.ErrInactiveUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				user.Status = database.UserStatusDormant
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
		{
			// Likewise, a system user must never be authenticated through
			// the delegated path.
			name:        "system user",
			expectedErr: aibridgedserver.ErrSystemUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				user.IsSystem = true
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(user, nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := testutil.Logger(t)

			now := dbtime.Now()
			user := database.User{
				ID:         uuid.New(),
				Email:      "test@coder.com",
				Username:   "test",
				Name:       "Test User",
				CreatedAt:  now,
				UpdatedAt:  now,
				RBACRoles:  []string{},
				LoginType:  database.LoginTypePassword,
				Status:     database.UserStatusActive,
				LastSeenAt: now,
			}
			keyID, _ := cryptorand.String(10)
			_, keySecretHashed, _ := apikey.GenerateSecret(22)
			apiKey := database.APIKey{
				ID:              keyID,
				LifetimeSeconds: 86400,
				HashedSecret:    keySecretHashed,
				UserID:          user.ID,
				LastUsed:        now,
				ExpiresAt:       now.Add(time.Hour),
				CreatedAt:       now,
				UpdatedAt:       now,
				LoginType:       database.LoginTypePassword,
				Scopes:          []database.APIKeyScope{database.ApiKeyScopeCoderAll},
			}

			if tc.mocksFn != nil {
				tc.mocksFn(db, apiKey, user)
			}

			srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{
				Store:         db,
				AISeatTracker: agplaiseats.Noop{},
				AccessURL:     "/",
				GatewayCfg:    codersdk.AIBridgeConfig{},
				Experiments:   requiredExperiments,
				Logger:        logger,
				Clock:         quartz.NewReal(),
			})
			require.NoError(t, err)
			require.NotNil(t, srv)

			req := &proto.IsAuthorizedRequest{KeyId: keyID}
			if tc.bothFields {
				req.Key = "anything-anything"
			}

			resp, err := srv.IsAuthorized(t.Context(), req)
			if tc.expectedErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, &proto.IsAuthorizedResponse{
				OwnerId:  user.ID.String(),
				ApiKeyId: keyID,
				Username: user.Username,
			}, resp)
		})
	}
}

func TestIsBudgetExceeded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		userIDStr         string
		setupMocks        func(db *dbmock.MockStore, userID uuid.UUID) (resp *proto.IsBudgetExceededResponse, blockedGroupID uuid.UUID)
		wantErrContains   string
		wantMetricOutcome string
	}{
		{
			// Invalid UUID short-circuits before any store call.
			name:              "invalid user_id",
			userIDStr:         "not-a-uuid",
			wantErrContains:   "invalid user_id",
			wantMetricOutcome: "error",
		},
		{
			// No override and no group budget resolves: pass-through.
			name: "no budget configured returns not exceeded",
			setupMocks: func(db *dbmock.MockStore, userID uuid.UUID) (*proto.IsBudgetExceededResponse, uuid.UUID) {
				db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), userID).
					Return(database.UserAIBudgetOverride{}, sql.ErrNoRows)
				db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), userID).
					Return(database.GetHighestGroupAIBudgetByUserRow{}, sql.ErrNoRows)
				return &proto.IsBudgetExceededResponse{
					Exceeded:         false,
					SpendLimitMicros: nil,
				}, uuid.Nil
			},
			wantMetricOutcome: "allowed",
		},
		{
			// Group budget resolves, spend below limit (spend 500 < limit 1000): pass-through.
			name: "under limit returns not exceeded",
			setupMocks: func(db *dbmock.MockStore, userID uuid.UUID) (*proto.IsBudgetExceededResponse, uuid.UUID) {
				groupID := uuid.New()
				db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), userID).
					Return(database.UserAIBudgetOverride{}, sql.ErrNoRows)
				db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), userID).
					Return(database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: 1_000}, nil)
				db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
					Return(database.GetUserAISpendSinceRow{SpendMicros: 500}, nil)
				return &proto.IsBudgetExceededResponse{
					Exceeded:         false,
					SpendLimitMicros: ptr.Ref(int64(1_000)),
				}, uuid.Nil
			},
			wantMetricOutcome: "allowed",
		},
		{
			// Group budget resolves, spend at limit (spend 1000 == limit 1000): blocked.
			name: "at limit returns exceeded",
			setupMocks: func(db *dbmock.MockStore, userID uuid.UUID) (*proto.IsBudgetExceededResponse, uuid.UUID) {
				groupID := uuid.New()
				db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), userID).
					Return(database.UserAIBudgetOverride{}, sql.ErrNoRows)
				db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), userID).
					Return(database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: 1_000}, nil)
				db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
					Return(database.GetUserAISpendSinceRow{SpendMicros: 1_000}, nil)
				return &proto.IsBudgetExceededResponse{
					Exceeded:         true,
					SpendLimitMicros: ptr.Ref(int64(1_000)),
				}, groupID
			},
			wantMetricOutcome: "blocked",
		},
		{
			// Limit of 0 is a valid "block-all" setting, distinct from
			// "no budget configured": blocked.
			name: "zero limit blocks all requests",
			setupMocks: func(db *dbmock.MockStore, userID uuid.UUID) (*proto.IsBudgetExceededResponse, uuid.UUID) {
				groupID := uuid.New()
				db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), userID).
					Return(database.UserAIBudgetOverride{}, sql.ErrNoRows)
				db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), userID).
					Return(database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: 0}, nil)
				db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
					Return(database.GetUserAISpendSinceRow{SpendMicros: 0}, nil)
				return &proto.IsBudgetExceededResponse{
					Exceeded:         true,
					SpendLimitMicros: ptr.Ref(int64(0)),
				}, groupID
			},
			wantMetricOutcome: "blocked",
		},
		{
			// Group budget resolves, spend above limit (spend 1500 > limit 1000): blocked.
			name: "over limit returns exceeded",
			setupMocks: func(db *dbmock.MockStore, userID uuid.UUID) (*proto.IsBudgetExceededResponse, uuid.UUID) {
				groupID := uuid.New()
				db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), userID).
					Return(database.UserAIBudgetOverride{}, sql.ErrNoRows)
				db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), userID).
					Return(database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: 1_000}, nil)
				db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
					Return(database.GetUserAISpendSinceRow{SpendMicros: 1_500}, nil)
				return &proto.IsBudgetExceededResponse{
					Exceeded:         true,
					SpendLimitMicros: ptr.Ref(int64(1_000)),
				}, groupID
			},
			wantMetricOutcome: "blocked",
		},
		{
			// User override wins, group lookup skipped, spend aggregated against
			// the override's group (spend 600 > limit 500): blocked.
			name: "user override wins over group budget",
			setupMocks: func(db *dbmock.MockStore, userID uuid.UUID) (*proto.IsBudgetExceededResponse, uuid.UUID) {
				overrideGroupID := uuid.New()
				db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), userID).
					Return(database.UserAIBudgetOverride{
						UserID:           userID,
						GroupID:          overrideGroupID,
						SpendLimitMicros: 500,
					}, nil)
				db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Cond(func(p database.GetUserAISpendSinceParams) bool {
					return assert.Equal(t, overrideGroupID, p.EffectiveGroupID, "spend aggregated against override group")
				})).Return(database.GetUserAISpendSinceRow{SpendMicros: 600}, nil)
				return &proto.IsBudgetExceededResponse{
					Exceeded:         true,
					SpendLimitMicros: ptr.Ref(int64(500)),
				}, overrideGroupID
			},
			wantMetricOutcome: "blocked",
		},
		{
			// Unexpected error from budget override lookup propagates.
			name: "budget resolution error propagates",
			setupMocks: func(db *dbmock.MockStore, userID uuid.UUID) (*proto.IsBudgetExceededResponse, uuid.UUID) {
				db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), userID).
					Return(database.UserAIBudgetOverride{}, sql.ErrConnDone)
				return nil, uuid.Nil
			},
			wantErrContains:   "resolve effective AI budget",
			wantMetricOutcome: "error",
		},
		{
			// Error from spend aggregation propagates (fail-closed).
			name: "spend aggregation error propagates",
			setupMocks: func(db *dbmock.MockStore, userID uuid.UUID) (*proto.IsBudgetExceededResponse, uuid.UUID) {
				db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), userID).
					Return(database.UserAIBudgetOverride{}, sql.ErrNoRows)
				db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), userID).
					Return(database.GetHighestGroupAIBudgetByUserRow{GroupID: uuid.New(), SpendLimitMicros: 1_000}, nil)
				db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
					Return(database.GetUserAISpendSinceRow{}, sql.ErrConnDone)
				return nil, uuid.Nil
			},
			wantErrContains:   "get user AI spend",
			wantMetricOutcome: "error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := testutil.Logger(t)

			userID := uuid.New()
			userIDStr := tc.userIDStr
			if userIDStr == "" {
				userIDStr = userID.String()
			}

			var wantResp *proto.IsBudgetExceededResponse
			var blockedGroupID uuid.UUID
			if tc.setupMocks != nil {
				wantResp, blockedGroupID = tc.setupMocks(db, userID)
			}

			reg := prometheus.NewRegistry()
			metrics := aibridgedserver.NewMetrics(reg)
			srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{
				Store:         db,
				AISeatTracker: agplaiseats.Noop{},
				AccessURL:     "/",
				GatewayCfg:    codersdk.AIBridgeConfig{},
				Experiments:   requiredExperiments,
				Logger:        logger,
				Clock:         quartz.NewReal(),
				Metrics:       metrics,
			})
			require.NoError(t, err)

			req := &proto.IsBudgetExceededRequest{UserId: userIDStr}
			resp, err := srv.IsBudgetExceeded(t.Context(), req)

			// The enforcement duration is always observed once, labeled by the
			// outcome, even when the check errors.
			require.Equal(t, 1, promtest.CollectAndCount(metrics.EnforcementDuration))
			require.EqualValues(t, 1, promhelp.HistogramValue(t, reg,
				"cost_control_enforcement_duration_seconds",
				prometheus.Labels{"outcome": tc.wantMetricOutcome}).GetSampleCount())
			wantBlocked := 0
			if tc.wantMetricOutcome == "blocked" {
				wantBlocked = 1
			}
			require.Equal(t, wantBlocked, promtest.CollectAndCount(metrics.BlockedRequests))
			if wantBlocked == 1 {
				require.Equal(t, 1, promhelp.CounterValue(t, reg,
					"cost_control_blocked_requests_total",
					prometheus.Labels{"group_id": blockedGroupID.String()}))
			}

			if tc.wantErrContains != "" {
				require.Error(t, err)
				require.Nil(t, resp)
				assert.ErrorContains(t, err, tc.wantErrContains)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, wantResp.GetExceeded(), resp.GetExceeded(), "exceeded")
			require.Equal(t, wantResp.SpendLimitMicros, resp.SpendLimitMicros, "spend_limit_micros")
		})
	}
}

// TestIsBudgetExceeded_Enforcement exercises real-DB scenarios that drive
// enforcement decisions.
func TestIsBudgetExceeded_Enforcement(t *testing.T) {
	t.Parallel()

	const groupLimitMicros = 1_000_000

	// setup provisions a user in an organization with a single budgeted group.
	setup := func(t *testing.T, clock quartz.Clock) (context.Context, database.Store, *aibridgedserver.Server, database.User, database.Group) {
		t.Helper()

		ctx := testutil.Context(t, testutil.WaitLong)
		logger := testutil.Logger(t)

		rawDB, _ := dbtestutil.NewDB(t)
		authzDB := dbauthz.New(rawDB, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())

		org := dbgen.Organization(t, rawDB, database.Organization{})
		user := dbgen.User(t, rawDB, database.User{})
		dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})
		group := dbgen.Group(t, rawDB, database.Group{OrganizationID: org.ID})
		dbgen.GroupMember(t, rawDB, database.GroupMemberTable{UserID: user.ID, GroupID: group.ID})

		_, err := rawDB.UpsertGroupAIBudget(ctx, database.UpsertGroupAIBudgetParams{
			GroupID:          group.ID,
			SpendLimitMicros: groupLimitMicros,
		})
		require.NoError(t, err)

		srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
			Store:         authzDB,
			AISeatTracker: agplaiseats.Noop{},
			AccessURL:     "/",
			GatewayCfg:    codersdk.AIBridgeConfig{},
			Experiments:   requiredExperiments,
			Logger:        logger,
			Clock:         clock,
		})
		require.NoError(t, err)

		return ctx, rawDB, srv, user, group
	}

	t.Run("period boundary excludes prior period spend", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		clock := quartz.NewMock(t)
		ctx, rawDB, srv, user, group := setup(t, clock)

		prevMonth := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
		nextMonth := time.Date(2026, time.February, 5, 0, 0, 0, 0, time.UTC)

		// Set now to 2026-01-15.
		clock.Set(prevMonth)

		// User spend on 2026-01-15 exceeds the group limit.
		_, err := rawDB.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID:           user.ID,
			EffectiveGroupID: group.ID,
			Day:              prevMonth,
			CostMicros:       1_500_000,
		})
		require.NoError(t, err)

		// Current period is January: includes the 2026-01-15 spend, user exceeded.
		prevMonthResp, err := srv.IsBudgetExceeded(ctx, &proto.IsBudgetExceededRequest{
			UserId: user.ID.String(),
		})
		require.NoError(t, err)
		require.True(t, prevMonthResp.GetExceeded())
		require.Equal(t, int64(groupLimitMicros), prevMonthResp.GetSpendLimitMicros())

		// Advance clock to 2026-02-05: excludes the 2026-01-15 spend, user not exceeded.
		clock.Set(nextMonth)
		nextMonthResp, err := srv.IsBudgetExceeded(ctx, &proto.IsBudgetExceededRequest{
			UserId: user.ID.String(),
		})
		require.NoError(t, err)
		require.False(t, nextMonthResp.GetExceeded())
		require.Equal(t, int64(groupLimitMicros), nextMonthResp.GetSpendLimitMicros())
	})

	t.Run("new user override unblocks user", func(t *testing.T) {
		t.Parallel()

		// Use fixed dates to keep the test deterministic.
		clock := quartz.NewMock(t)
		ctx, rawDB, srv, user, group := setup(t, clock)

		now := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)

		// Set now to 2026-03-15.
		clock.Set(now)

		// User spend on 2026-03-15 exceeds the group limit.
		_, err := rawDB.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID:           user.ID,
			EffectiveGroupID: group.ID,
			Day:              now,
			CostMicros:       1_500_000,
		})
		require.NoError(t, err)

		// User's spend exceeds the group limit.
		beforeResp, err := srv.IsBudgetExceeded(ctx, &proto.IsBudgetExceededRequest{
			UserId: user.ID.String(),
		})
		require.NoError(t, err)
		require.True(t, beforeResp.GetExceeded())
		require.Equal(t, int64(groupLimitMicros), beforeResp.GetSpendLimitMicros())

		// Add user override with a higher limit on the same group. The override
		// wins, so the user's spend is now under the effective limit.
		const overrideLimitMicros = 2_000_000
		_, err = rawDB.UpsertUserAIBudgetOverride(ctx, database.UpsertUserAIBudgetOverrideParams{
			UserID:           user.ID,
			GroupID:          group.ID,
			SpendLimitMicros: overrideLimitMicros,
		})
		require.NoError(t, err)

		afterResp, err := srv.IsBudgetExceeded(ctx, &proto.IsBudgetExceededRequest{
			UserId: user.ID.String(),
		})
		require.NoError(t, err)
		require.False(t, afterResp.GetExceeded())
		require.Equal(t, int64(overrideLimitMicros), afterResp.GetSpendLimitMicros())
	})

	t.Run("unbudgeted member is not blocked", func(t *testing.T) {
		t.Parallel()

		clock := quartz.NewMock(t)
		clock.Set(time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC))

		ctx := testutil.Context(t, testutil.WaitLong)
		logger := testutil.Logger(t)
		rawDB, _ := dbtestutil.NewDB(t)
		authzDB := dbauthz.New(rawDB, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())

		// An org member with no group budget and no override: spend is
		// unlimited, so enforcement never blocks them.
		org := dbgen.Organization(t, rawDB, database.Organization{})
		user := dbgen.User(t, rawDB, database.User{})
		dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})

		// Record spend attributed to the Everyone group. Without a configured
		// limit it must not cause a block.
		_, err := rawDB.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
			UserID:           user.ID,
			EffectiveGroupID: org.ID,
			Day:              clock.Now(),
			CostMicros:       1_000_000_000, // $1,000 USD
		})
		require.NoError(t, err)

		srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
			Store:         authzDB,
			AISeatTracker: agplaiseats.Noop{},
			AccessURL:     "/",
			GatewayCfg:    codersdk.AIBridgeConfig{},
			Experiments:   requiredExperiments,
			Logger:        logger,
			Clock:         clock,
		})
		require.NoError(t, err)

		resp, err := srv.IsBudgetExceeded(ctx, &proto.IsBudgetExceededRequest{UserId: user.ID.String()})
		require.NoError(t, err)
		require.False(t, resp.GetExceeded())
		require.Nil(t, resp.SpendLimitMicros)
	})
}

func TestGetMCPServerConfigs(t *testing.T) {
	t.Parallel()

	externalAuthCfgs := []*externalauth.Config{
		{
			ID:     "1",
			MCPURL: "1.com/mcp",
		},
		{
			ID: "2", // Will not be eligible for inclusion since MCPURL is not defined.
		},
	}

	cases := []struct {
		name                     string
		disableCoderMCPInjection bool
		experiments              codersdk.Experiments
		externalAuthConfigs      []*externalauth.Config
		expectCoderMCP           bool
		expectedExternalMCP      bool
	}{
		{
			name:        "experiments not enabled",
			experiments: codersdk.Experiments{},
		},
		{
			name:        "MCP experiment enabled, not OAuth2",
			experiments: codersdk.Experiments{codersdk.ExperimentMCPServerHTTP},
		},
		{
			name:        "OAuth2 experiment enabled, not MCP",
			experiments: codersdk.Experiments{codersdk.ExperimentOAuth2},
		},
		{
			name:           "only internal MCP",
			experiments:    requiredExperiments,
			expectCoderMCP: true,
		},
		{
			name:                "only external MCP",
			externalAuthConfigs: externalAuthCfgs,
			expectedExternalMCP: true,
		},
		{
			name:                "both internal & external MCP",
			experiments:         requiredExperiments,
			externalAuthConfigs: externalAuthCfgs,
			expectCoderMCP:      true,
			expectedExternalMCP: true,
		},
		{
			name:                     "both internal & external MCP, but coder MCP tools not injected",
			disableCoderMCPInjection: true,
			experiments:              requiredExperiments,
			externalAuthConfigs:      externalAuthCfgs,
			expectCoderMCP:           false,
			expectedExternalMCP:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := testutil.Logger(t)

			accessURL := "https://my-cool-deployment.com"
			srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{
				Store:         db,
				AISeatTracker: agplaiseats.Noop{},
				AccessURL:     accessURL,
				GatewayCfg: codersdk.AIBridgeConfig{
					InjectCoderMCPTools: serpent.Bool(!tc.disableCoderMCPInjection),
				},
				ExternalAuthConfigs: tc.externalAuthConfigs,
				Experiments:         tc.experiments,
				Logger:              logger,
				Clock:               quartz.NewReal(),
			})
			require.NoError(t, err)
			require.NotNil(t, srv)

			resp, err := srv.GetMCPServerConfigs(t.Context(), &proto.GetMCPServerConfigsRequest{})
			require.NoError(t, err)
			require.NotNil(t, resp)

			if tc.expectCoderMCP {
				coderConfig := resp.CoderMcpConfig
				require.NotNil(t, coderConfig)
				require.Equal(t, aibridged.InternalMCPServerID, coderConfig.GetId())
				expectedURL, err := url.JoinPath(accessURL, codermcp.MCPEndpoint)
				require.NoError(t, err)
				require.Equal(t, expectedURL, coderConfig.GetUrl())
				require.Empty(t, coderConfig.GetToolAllowRegex())
				require.Empty(t, coderConfig.GetToolDenyRegex())
			} else {
				require.Empty(t, resp.GetCoderMcpConfig())
			}

			if tc.expectedExternalMCP {
				require.Len(t, resp.GetExternalAuthMcpConfigs(), 1)
			} else {
				require.Empty(t, resp.GetExternalAuthMcpConfigs())
			}
		})
	}
}

func TestGetMCPServerAccessTokensBatch(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := testutil.Logger(t)

	// Given: 2 external auth configured with MCP and 1 without.
	srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{
		Store:         db,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		ExternalAuthConfigs: []*externalauth.Config{
			{
				ID:     "1",
				MCPURL: "1.com/mcp",
			},
			{
				ID:     "2",
				MCPURL: "2.com/mcp",
			},
			{
				ID: "3",
			},
		},
		Experiments: requiredExperiments,
		Logger:      logger,
		Clock:       quartz.NewReal(),
	})
	require.NoError(t, err)
	require.NotNil(t, srv)

	// When: requesting all external auth links, return all.
	db.EXPECT().GetExternalAuthLinksByUserID(gomock.Any(), gomock.Any()).MinTimes(1).DoAndReturn(func(ctx context.Context, userID uuid.UUID) ([]database.ExternalAuthLink, error) {
		return []database.ExternalAuthLink{
			{
				UserID:           userID,
				ProviderID:       "1",
				OAuthAccessToken: "1-token",
			},
			{
				UserID:           userID,
				ProviderID:       "2",
				OAuthAccessToken: "2-token",
				OAuthExpiry:      dbtime.Now().Add(-time.Minute), // This token is expired and should not be returned.
			},
			{
				UserID:           userID,
				ProviderID:       "3",
				OAuthAccessToken: "3-token",
			},
		}, nil
	})

	// When: accessing the MCP server access tokens, only the 2 with MCP configured should be returned, and the 1 without should
	// not fail the request but rather have an error returned specifically for that server.
	resp, err := srv.GetMCPServerAccessTokensBatch(t.Context(), &proto.GetMCPServerAccessTokensBatchRequest{
		UserId:             uuid.NewString(),
		McpServerConfigIds: []string{"1", "1", "2", "3"}, // Duplicates must be tolerated.
	})
	require.NoError(t, err)

	// Then: 2 MCP servers are eligible but only 1 will return a valid token as the other expired.
	require.Len(t, resp.GetAccessTokens(), 1)
	require.Equal(t, "1-token", resp.GetAccessTokens()["1"])
	require.Len(t, resp.GetErrors(), 2)
	require.Contains(t, resp.GetErrors()["2"], aibridgedserver.ErrExpiredOrInvalidOAuthToken.Error())
	require.Contains(t, resp.GetErrors()["3"], aibridgedserver.ErrNoMCPConfigFound.Error())
}

func TestRecordInterception(t *testing.T) {
	t.Parallel()

	var (
		metadataProto = map[string]*anypb.Any{
			"key": mustMarshalAny(t, &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "value"}}),
		}
		metadataJSON = `{"key":"value"}`
	)

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
			return srv.RecordInterception(ctx, req)
		},
		[]testRecordMethodCase[*proto.RecordInterceptionRequest]{
			{
				name: "valid interception",
				request: &proto.RecordInterceptionRequest{
					Id:             uuid.NewString(),
					ApiKeyId:       uuid.NewString(),
					InitiatorId:    uuid.NewString(),
					Provider:       "anthropic",
					ProviderName:   "anthropic",
					Model:          "claude-4-opus",
					Metadata:       metadataProto,
					StartedAt:      timestamppb.Now(),
					CredentialKind: "byok",
					CredentialHint: "sk-a...efgh",
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")
					initiatorID, err := uuid.Parse(req.GetInitiatorId())
					assert.NoError(t, err, "parse interception initiator UUID")

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), database.InsertAIBridgeInterceptionParams{
						ID:             interceptionID,
						APIKeyID:       sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:    initiatorID,
						Provider:       req.GetProvider(),
						ProviderName:   req.GetProviderName(),
						Model:          req.GetModel(),
						Metadata:       json.RawMessage(metadataJSON),
						StartedAt:      req.StartedAt.AsTime().UTC(),
						CredentialKind: database.CredentialKindByok,
						CredentialHint: "sk-a...efgh",
					}).Return(database.AIBridgeInterception{
						ID:             interceptionID,
						APIKeyID:       sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:    initiatorID,
						Provider:       req.GetProvider(),
						ProviderName:   req.GetProviderName(),
						Model:          req.GetModel(),
						StartedAt:      req.StartedAt.AsTime().UTC(),
						CredentialKind: database.CredentialKindByok,
						CredentialHint: "sk-a...efgh",
					}, nil)
				},
			},
			{
				name: "valid interception with client session ID",
				request: &proto.RecordInterceptionRequest{
					Id:              uuid.NewString(),
					ApiKeyId:        uuid.NewString(),
					InitiatorId:     uuid.NewString(),
					Provider:        "anthropic",
					Model:           "claude-4-opus",
					Metadata:        metadataProto,
					StartedAt:       timestamppb.Now(),
					ClientSessionId: ptr.Ref("session-abc-123"),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")
					initiatorID, err := uuid.Parse(req.GetInitiatorId())
					assert.NoError(t, err, "parse interception initiator UUID")

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), database.InsertAIBridgeInterceptionParams{
						ID:              interceptionID,
						APIKeyID:        sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:     initiatorID,
						Provider:        req.GetProvider(),
						ProviderName:    req.GetProvider(),
						Model:           req.GetModel(),
						Metadata:        json.RawMessage(metadataJSON),
						StartedAt:       req.StartedAt.AsTime().UTC(),
						ClientSessionID: sql.NullString{String: "session-abc-123", Valid: true},
						CredentialKind:  database.CredentialKindCentralized,
					}).Return(database.AIBridgeInterception{
						ID:              interceptionID,
						APIKeyID:        sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:     initiatorID,
						Provider:        req.GetProvider(),
						ProviderName:    req.GetProvider(),
						Model:           req.GetModel(),
						StartedAt:       req.StartedAt.AsTime().UTC(),
						ClientSessionID: sql.NullString{String: "session-abc-123", Valid: true},
					}, nil)
				},
			},
			{
				name: "empty client session ID treated as null",
				request: &proto.RecordInterceptionRequest{
					Id:              uuid.NewString(),
					ApiKeyId:        uuid.NewString(),
					InitiatorId:     uuid.NewString(),
					Provider:        "anthropic",
					Model:           "claude-4-opus",
					Metadata:        metadataProto,
					StartedAt:       timestamppb.Now(),
					ClientSessionId: ptr.Ref(""),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")
					initiatorID, err := uuid.Parse(req.GetInitiatorId())
					assert.NoError(t, err, "parse interception initiator UUID")

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), database.InsertAIBridgeInterceptionParams{
						ID:              interceptionID,
						APIKeyID:        sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:     initiatorID,
						Provider:        req.GetProvider(),
						ProviderName:    req.GetProvider(),
						Model:           req.GetModel(),
						Metadata:        json.RawMessage(metadataJSON),
						StartedAt:       req.StartedAt.AsTime().UTC(),
						ClientSessionID: sql.NullString{},
						CredentialKind:  database.CredentialKindCentralized,
					}).Return(database.AIBridgeInterception{
						ID:           interceptionID,
						APIKeyID:     sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:  initiatorID,
						Provider:     req.GetProvider(),
						ProviderName: req.GetProvider(),
						Model:        req.GetModel(),
						StartedAt:    req.StartedAt.AsTime().UTC(),
					}, nil)
				},
			},
			{
				name: "valid interception with agent firewall correlation",
				request: &proto.RecordInterceptionRequest{
					Id:                          uuid.NewString(),
					ApiKeyId:                    uuid.NewString(),
					InitiatorId:                 uuid.NewString(),
					Provider:                    "anthropic",
					Model:                       "claude-4-opus",
					Metadata:                    metadataProto,
					StartedAt:                   timestamppb.Now(),
					AgentFirewallSessionId:      ptr.Ref(uuid.NewString()),
					AgentFirewallSequenceNumber: ptr.Ref(int32(42)),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")
					initiatorID, err := uuid.Parse(req.GetInitiatorId())
					assert.NoError(t, err, "parse interception initiator UUID")
					agentFirewallSessionID, err := uuid.Parse(req.GetAgentFirewallSessionId())
					assert.NoError(t, err, "parse agent firewall session UUID")

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), database.InsertAIBridgeInterceptionParams{
						ID:                          interceptionID,
						APIKeyID:                    sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:                 initiatorID,
						Provider:                    req.GetProvider(),
						ProviderName:                req.GetProvider(),
						Model:                       req.GetModel(),
						Metadata:                    json.RawMessage(metadataJSON),
						StartedAt:                   req.StartedAt.AsTime().UTC(),
						CredentialKind:              database.CredentialKindCentralized,
						AgentFirewallSessionID:      uuid.NullUUID{UUID: agentFirewallSessionID, Valid: true},
						AgentFirewallSequenceNumber: sql.NullInt32{Int32: 42, Valid: true},
					}).Return(database.AIBridgeInterception{
						ID:          interceptionID,
						InitiatorID: initiatorID,
						Provider:    req.GetProvider(),
						Model:       req.GetModel(),
						StartedAt:   req.StartedAt.AsTime().UTC(),
					}, nil)
				},
			},
			{
				name: "absent agent firewall fields treated as null",
				request: &proto.RecordInterceptionRequest{
					Id:          uuid.NewString(),
					ApiKeyId:    uuid.NewString(),
					InitiatorId: uuid.NewString(),
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					Metadata:    metadataProto,
					StartedAt:   timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")
					initiatorID, err := uuid.Parse(req.GetInitiatorId())
					assert.NoError(t, err, "parse interception initiator UUID")

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), database.InsertAIBridgeInterceptionParams{
						ID:                          interceptionID,
						APIKeyID:                    sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:                 initiatorID,
						Provider:                    req.GetProvider(),
						ProviderName:                req.GetProvider(),
						Model:                       req.GetModel(),
						Metadata:                    json.RawMessage(metadataJSON),
						StartedAt:                   req.StartedAt.AsTime().UTC(),
						CredentialKind:              database.CredentialKindCentralized,
						AgentFirewallSessionID:      uuid.NullUUID{},
						AgentFirewallSequenceNumber: sql.NullInt32{},
					}).Return(database.AIBridgeInterception{
						ID:          interceptionID,
						InitiatorID: initiatorID,
						Provider:    req.GetProvider(),
						Model:       req.GetModel(),
						StartedAt:   req.StartedAt.AsTime().UTC(),
					}, nil)
				},
			},
			{
				name: "invalid agent firewall session ID treated as null",
				request: &proto.RecordInterceptionRequest{
					Id:                          uuid.NewString(),
					ApiKeyId:                    uuid.NewString(),
					InitiatorId:                 uuid.NewString(),
					Provider:                    "anthropic",
					Model:                       "claude-4-opus",
					Metadata:                    metadataProto,
					StartedAt:                   timestamppb.Now(),
					AgentFirewallSessionId:      ptr.Ref("not-a-uuid"),
					AgentFirewallSequenceNumber: ptr.Ref(int32(7)),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")
					initiatorID, err := uuid.Parse(req.GetInitiatorId())
					assert.NoError(t, err, "parse interception initiator UUID")

					// Malformed agent firewall session ID is stored as null
					// (and logged) rather than failing the interception.
					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), database.InsertAIBridgeInterceptionParams{
						ID:                          interceptionID,
						APIKeyID:                    sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:                 initiatorID,
						Provider:                    req.GetProvider(),
						ProviderName:                req.GetProvider(),
						Model:                       req.GetModel(),
						Metadata:                    json.RawMessage(metadataJSON),
						StartedAt:                   req.StartedAt.AsTime().UTC(),
						CredentialKind:              database.CredentialKindCentralized,
						AgentFirewallSessionID:      uuid.NullUUID{},
						AgentFirewallSequenceNumber: sql.NullInt32{Int32: 7, Valid: true},
					}).Return(database.AIBridgeInterception{
						ID:          interceptionID,
						InitiatorID: initiatorID,
						Provider:    req.GetProvider(),
						Model:       req.GetModel(),
						StartedAt:   req.StartedAt.AsTime().UTC(),
					}, nil)
				},
			},
			{
				name: "invalid interception ID",
				request: &proto.RecordInterceptionRequest{
					Id:          "not-a-uuid",
					InitiatorId: uuid.NewString(),
					ApiKeyId:    uuid.NewString(),
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					StartedAt:   timestamppb.Now(),
				},
				expectedErr: "invalid interception ID",
			},
			{
				name: "invalid initiator ID",
				request: &proto.RecordInterceptionRequest{
					Id:          uuid.NewString(),
					ApiKeyId:    uuid.NewString(),
					InitiatorId: "not-a-uuid",
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					StartedAt:   timestamppb.Now(),
				},
				expectedErr: "invalid initiator ID",
			},
			{
				name: "invalid interception no api key set",
				request: &proto.RecordInterceptionRequest{
					Id:          uuid.NewString(),
					InitiatorId: uuid.NewString(),
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					Metadata:    metadataProto,
					StartedAt:   timestamppb.Now(),
				},
				expectedErr: "empty API key ID",
			},
			{
				name: "provider name differs from provider type",
				request: &proto.RecordInterceptionRequest{
					Id:           uuid.NewString(),
					ApiKeyId:     uuid.NewString(),
					InitiatorId:  uuid.NewString(),
					Provider:     "copilot",
					ProviderName: "copilot-business",
					Model:        "gpt-4o",
					StartedAt:    timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")
					initiatorID, err := uuid.Parse(req.GetInitiatorId())
					assert.NoError(t, err, "parse interception initiator UUID")

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), database.InsertAIBridgeInterceptionParams{
						ID:             interceptionID,
						APIKeyID:       sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:    initiatorID,
						Provider:       "copilot",
						ProviderName:   "copilot-business",
						Model:          req.GetModel(),
						Metadata:       json.RawMessage("{}"),
						StartedAt:      req.StartedAt.AsTime().UTC(),
						CredentialKind: database.CredentialKindCentralized,
					}).Return(database.AIBridgeInterception{
						ID:           interceptionID,
						InitiatorID:  initiatorID,
						Provider:     "copilot",
						ProviderName: "copilot-business",
						Model:        req.GetModel(),
						StartedAt:    req.StartedAt.AsTime().UTC(),
					}, nil)
				},
			},
			{
				name: "empty provider name defaults to provider",
				request: &proto.RecordInterceptionRequest{
					Id:          uuid.NewString(),
					ApiKeyId:    uuid.NewString(),
					InitiatorId: uuid.NewString(),
					Provider:    "copilot",
					Model:       "gpt-4o",
					StartedAt:   timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")
					initiatorID, err := uuid.Parse(req.GetInitiatorId())
					assert.NoError(t, err, "parse interception initiator UUID")

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), database.InsertAIBridgeInterceptionParams{
						ID:             interceptionID,
						APIKeyID:       sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:    initiatorID,
						Provider:       "copilot",
						ProviderName:   "copilot",
						Model:          req.GetModel(),
						Metadata:       json.RawMessage("{}"),
						StartedAt:      req.StartedAt.AsTime().UTC(),
						CredentialKind: database.CredentialKindCentralized,
					}).Return(database.AIBridgeInterception{
						ID:           interceptionID,
						InitiatorID:  initiatorID,
						Provider:     "copilot",
						ProviderName: "copilot",
						Model:        req.GetModel(),
						StartedAt:    req.StartedAt.AsTime().UTC(),
					}, nil)
				},
			},
			{
				name: "whitespace provider name defaults to provider",
				request: &proto.RecordInterceptionRequest{
					Id:           uuid.NewString(),
					ApiKeyId:     uuid.NewString(),
					InitiatorId:  uuid.NewString(),
					Provider:     "copilot",
					ProviderName: "   ",
					Model:        "gpt-4o",
					StartedAt:    timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")
					initiatorID, err := uuid.Parse(req.GetInitiatorId())
					assert.NoError(t, err, "parse interception initiator UUID")

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), database.InsertAIBridgeInterceptionParams{
						ID:             interceptionID,
						APIKeyID:       sql.NullString{String: req.ApiKeyId, Valid: true},
						InitiatorID:    initiatorID,
						Provider:       "copilot",
						ProviderName:   "copilot",
						Model:          req.GetModel(),
						Metadata:       json.RawMessage("{}"),
						StartedAt:      req.StartedAt.AsTime().UTC(),
						CredentialKind: database.CredentialKindCentralized,
					}).Return(database.AIBridgeInterception{
						ID:           interceptionID,
						InitiatorID:  initiatorID,
						Provider:     "copilot",
						ProviderName: "copilot",
						Model:        req.GetModel(),
						StartedAt:    req.StartedAt.AsTime().UTC(),
					}, nil)
				},
			},
			{
				name: "database error",
				request: &proto.RecordInterceptionRequest{
					Id:          uuid.NewString(),
					ApiKeyId:    uuid.NewString(),
					InitiatorId: uuid.NewString(),
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					StartedAt:   timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Any()).Return(database.AIBridgeInterception{}, sql.ErrConnDone)
				},
				expectedErr: "start interception",
			},
			{
				name: "ok with parent correlation",
				request: &proto.RecordInterceptionRequest{
					Id:                    uuid.UUID{3}.String(),
					ApiKeyId:              uuid.NewString(),
					InitiatorId:           uuid.NewString(),
					Provider:              "anthropic",
					Model:                 "claude-4-opus",
					StartedAt:             timestamppb.Now(),
					CorrelatingToolCallId: ptr.Ref("call_abc"),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					selfID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse self UUID")
					parentID := uuid.UUID{4}
					rootID := uuid.UUID{5}

					db.EXPECT().GetAIBridgeInterceptionLineageByToolCallID(
						gomock.Any(),
						"call_abc",
					).Return(database.GetAIBridgeInterceptionLineageByToolCallIDRow{
						ThreadParentID: parentID,
						ThreadRootID:   rootID,
					}, nil)

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeInterceptionParams) bool {
						return assert.Equal(t, selfID, p.ID, "ID") &&
							assert.Equal(t, uuid.NullUUID{UUID: parentID, Valid: true}, p.ThreadParentInterceptionID, "thread parent interception ID") &&
							assert.Equal(t, uuid.NullUUID{UUID: rootID, Valid: true}, p.ThreadRootInterceptionID, "thread root interception ID")
					})).Return(database.AIBridgeInterception{
						ID: selfID,
					}, nil)
				},
			},
			{
				name: "no lineage",
				request: &proto.RecordInterceptionRequest{
					Id:                    uuid.UUID{3}.String(),
					ApiKeyId:              uuid.NewString(),
					InitiatorId:           uuid.NewString(),
					Provider:              "anthropic",
					Model:                 "claude-4-opus",
					StartedAt:             timestamppb.Now(),
					CorrelatingToolCallId: ptr.Ref("call_abc"),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					selfID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse self UUID")

					db.EXPECT().GetAIBridgeInterceptionLineageByToolCallID(
						gomock.Any(),
						"call_abc",
					).Return(database.GetAIBridgeInterceptionLineageByToolCallIDRow{}, sql.ErrNoRows)

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeInterceptionParams) bool {
						return assert.Equal(t, selfID, p.ID, "ID") &&
							assert.Equal(t, uuid.NullUUID{}, p.ThreadParentInterceptionID, "thread parent interception ID") &&
							assert.Equal(t, uuid.NullUUID{}, p.ThreadRootInterceptionID, "thread root interception ID")
					})).Return(database.AIBridgeInterception{
						ID: selfID,
					}, nil)
				},
			},
			{
				name: "parent without root", // This should never happen since GetAIBridgeInterceptionLineageByToolCallID always returns both, but still...
				request: &proto.RecordInterceptionRequest{
					Id:                    uuid.UUID{3}.String(),
					ApiKeyId:              uuid.NewString(),
					InitiatorId:           uuid.NewString(),
					Provider:              "anthropic",
					Model:                 "claude-4-opus",
					StartedAt:             timestamppb.Now(),
					CorrelatingToolCallId: ptr.Ref("call_abc"),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					selfID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse self UUID")
					parentID := uuid.UUID{4}

					db.EXPECT().GetAIBridgeInterceptionLineageByToolCallID(
						gomock.Any(),
						"call_abc",
					).Return(database.GetAIBridgeInterceptionLineageByToolCallIDRow{
						ThreadParentID: parentID,
					}, nil)

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeInterceptionParams) bool {
						return assert.Equal(t, selfID, p.ID, "ID") &&
							assert.Equal(t, uuid.NullUUID{UUID: parentID, Valid: true}, p.ThreadParentInterceptionID, "thread parent interception ID") &&
							assert.Equal(t, uuid.NullUUID{}, p.ThreadRootInterceptionID, "thread root interception ID not expected")
					})).Return(database.AIBridgeInterception{
						ID: selfID,
					}, nil)
				},
			},
			{
				name: "ok no parent found",
				request: &proto.RecordInterceptionRequest{
					Id:                    uuid.UUID{5}.String(),
					ApiKeyId:              uuid.NewString(),
					InitiatorId:           uuid.NewString(),
					Provider:              "anthropic",
					Model:                 "claude-4-opus",
					StartedAt:             timestamppb.Now(),
					CorrelatingToolCallId: ptr.Ref("call_orphan"),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					selfID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse self UUID")

					db.EXPECT().GetAIBridgeInterceptionLineageByToolCallID(
						gomock.Any(),
						"call_orphan",
					).Return(database.GetAIBridgeInterceptionLineageByToolCallIDRow{}, sql.ErrNoRows)

					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeInterceptionParams) bool {
						return assert.Equal(t, selfID, p.ID, "ID") &&
							assert.Equal(t, uuid.NullUUID{}, p.ThreadParentInterceptionID, "thread parent interception ID") &&
							assert.Equal(t, uuid.NullUUID{}, p.ThreadRootInterceptionID, "thread root interception ID")
					})).Return(database.AIBridgeInterception{
						ID: selfID,
					}, nil)
				},
			},
		},
	)
}

func TestRecordInterceptionEnded(t *testing.T) {
	t.Parallel()

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordInterceptionEndedRequest) (*proto.RecordInterceptionEndedResponse, error) {
			return srv.RecordInterceptionEnded(ctx, req)
		},
		[]testRecordMethodCase[*proto.RecordInterceptionEndedRequest]{
			{
				name: "ok",
				request: &proto.RecordInterceptionEndedRequest{
					Id:             uuid.UUID{1}.String(),
					EndedAt:        timestamppb.Now(),
					CredentialHint: "sk-a...efgh",
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionEndedRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")

					db.EXPECT().UpdateAIBridgeInterceptionEnded(gomock.Any(), database.UpdateAIBridgeInterceptionEndedParams{
						ID:             interceptionID,
						EndedAt:        req.EndedAt.AsTime(),
						CredentialHint: req.CredentialHint,
					}).Return(database.AIBridgeInterception{
						ID:             interceptionID,
						InitiatorID:    uuid.UUID{2},
						Provider:       "prov",
						Model:          "mod",
						StartedAt:      time.Now(),
						EndedAt:        sql.NullTime{Time: req.EndedAt.AsTime(), Valid: true},
						CredentialHint: req.CredentialHint,
					}, nil)
				},
			},
			{
				name: "ok_with_error",
				request: &proto.RecordInterceptionEndedRequest{
					Id:           uuid.UUID{1}.String(),
					EndedAt:      timestamppb.Now(),
					ErrorType:    protobufproto.String(string(database.AibridgeInterceptionErrorTypeRateLimited)),
					ErrorMessage: protobufproto.String("rate limited by upstream"),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionEndedRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")

					db.EXPECT().UpdateAIBridgeInterceptionEnded(gomock.Any(), database.UpdateAIBridgeInterceptionEndedParams{
						ID:      interceptionID,
						EndedAt: req.EndedAt.AsTime(),
						ErrorType: database.NullAIBridgeInterceptionErrorType{
							AIBridgeInterceptionErrorType: database.AIBridgeInterceptionErrorType(req.GetErrorType()),
							Valid:                         true,
						},
						ErrorMessage: sql.NullString{String: req.GetErrorMessage(), Valid: true},
					}).Return(database.AIBridgeInterception{ID: interceptionID}, nil)
				},
			},
			{
				name: "invalid_error_type_is_unknown",
				request: &proto.RecordInterceptionEndedRequest{
					Id:        uuid.UUID{1}.String(),
					EndedAt:   timestamppb.Now(),
					ErrorType: protobufproto.String("not-a-real-type"),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionEndedRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")

					// A non-empty but unrecognized error type is stored as
					// 'unknown' (not NULL), keeping the error columns consistent.
					db.EXPECT().UpdateAIBridgeInterceptionEnded(gomock.Any(), database.UpdateAIBridgeInterceptionEndedParams{
						ID:      interceptionID,
						EndedAt: req.EndedAt.AsTime(),
						ErrorType: database.NullAIBridgeInterceptionErrorType{
							AIBridgeInterceptionErrorType: database.AibridgeInterceptionErrorTypeUnknown,
							Valid:                         true,
						},
					}).Return(database.AIBridgeInterception{ID: interceptionID}, nil)
				},
			},
			{
				name: "message_without_error_type_stores_neither",
				request: &proto.RecordInterceptionEndedRequest{
					Id:           uuid.UUID{1}.String(),
					EndedAt:      timestamppb.Now(),
					ErrorMessage: protobufproto.String("orphan message with no type"),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionEndedRequest) {
					interceptionID, err := uuid.Parse(req.GetId())
					assert.NoError(t, err, "parse interception UUID")

					// A message without a type is not a categorized error, so
					// both columns stay NULL to preserve the both-NULL == success
					// invariant rather than persisting a half-populated error.
					db.EXPECT().UpdateAIBridgeInterceptionEnded(gomock.Any(), database.UpdateAIBridgeInterceptionEndedParams{
						ID:           interceptionID,
						EndedAt:      req.EndedAt.AsTime(),
						ErrorType:    database.NullAIBridgeInterceptionErrorType{},
						ErrorMessage: sql.NullString{},
					}).Return(database.AIBridgeInterception{ID: interceptionID}, nil)
				},
			},
			{
				name: "bad_uuid_error",
				request: &proto.RecordInterceptionEndedRequest{
					Id: "this-is-not-uuid",
				},
				setupMocks:  func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionEndedRequest) {},
				expectedErr: "invalid interception ID",
			},
			{
				name: "database_error",
				request: &proto.RecordInterceptionEndedRequest{
					Id:      uuid.UUID{1}.String(),
					EndedAt: timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionEndedRequest) {
					db.EXPECT().UpdateAIBridgeInterceptionEnded(gomock.Any(), gomock.Any()).Return(database.AIBridgeInterception{}, sql.ErrConnDone)
				},
				expectedErr: "end interception: " + sql.ErrConnDone.Error(),
			},
		},
	)
}

func TestRecordTokenUsage(t *testing.T) {
	t.Parallel()

	var (
		metadataProto = map[string]*anypb.Any{
			"key": mustMarshalAny(t, &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "value"}}),
		}
		metadataJSON = `{"key":"value"}`
		// Use fixed dates to keep the test deterministic.
		now = time.Date(2026, 6, 25, 14, 30, 0, 0, time.UTC)
	)

	// Budget resolution falls through to the Everyone group, for cases that vary
	// only provider resolution.
	expectBudgetLookups := func(db *dbmock.MockStore, intc database.AIBridgeInterception) {
		db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intc.ID).Return(intc, nil)
		db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), intc.InitiatorID).
			Return(database.UserAIBudgetOverride{}, sql.ErrNoRows)
		db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), intc.InitiatorID).
			Return(database.GetHighestGroupAIBudgetByUserRow{}, sql.ErrNoRows)
		db.EXPECT().GetUserEveryoneFallbackGroup(gomock.Any(), intc.InitiatorID).Return(uuid.New(), nil)
	}

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordTokenUsageRequest) (*proto.RecordTokenUsageResponse, error) {
			return srv.RecordTokenUsage(ctx, req)
		},
		[]testRecordMethodCase[*proto.RecordTokenUsageRequest]{
			{
				// Budget resolves via group lookup, model is priced.
				name: "valid token usage with effective group and cost",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId:        uuid.NewString(),
					MsgId:                 "msg_123",
					InputTokens:           100,
					OutputTokens:          200,
					CacheReadInputTokens:  50,
					CacheWriteInputTokens: 10,
					CreatedAt:             timestamppb.New(now),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					groupID := uuid.New()
					group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: 1_000_000_000}
					price := &database.AIModelPrice{
						Provider:        intc.Provider,
						Model:           intc.Model,
						InputPrice:      sql.NullInt64{Int64: 3_000_000, Valid: true},
						OutputPrice:     sql.NullInt64{Int64: 6_000_000, Valid: true},
						CacheReadPrice:  sql.NullInt64{Int64: 300_000, Valid: true},
						CacheWritePrice: sql.NullInt64{Int64: 4_000_000, Valid: true},
					}
					// No override
					expectTokenUsageCostLookups(db, intc, nil, group, nil, price)

					// input 300 + output 1200 + cache read 15 + cache write 40.
					const wantCost int64 = 1555

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)

					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						if !assert.Equal(t, uuid.NullUUID{UUID: groupID, Valid: true}, p.EffectiveGroupID, "effective group ID") ||
							!assert.Equal(t, price.InputPrice, p.InputPriceMicros, "input price") ||
							!assert.Equal(t, price.OutputPrice, p.OutputPriceMicros, "output price") ||
							!assert.Equal(t, price.CacheReadPrice, p.CacheReadPriceMicros, "cache read price") ||
							!assert.Equal(t, price.CacheWritePrice, p.CacheWritePriceMicros, "cache write price") ||
							!assert.Equal(t, sql.NullInt64{Int64: wantCost, Valid: true}, p.CostMicros, "cost") {
							return false
						}
						return true
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)

					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), database.IncrementUserAIDailySpendParams{
						UserID:           intc.InitiatorID,
						EffectiveGroupID: groupID,
						Day:              now.UTC().Truncate(24 * time.Hour),
						CostMicros:       wantCost,
					}).Return(database.AIUserDailySpend{}, nil)

					db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
						Return(database.GetUserAISpendSinceRow{SpendMicros: wantCost}, nil)
				},
				// A priced model does not increment unpriced_token_usage_records_total.
				assertMetrics: func(t *testing.T, reg *prometheus.Registry) {
					require.Nil(t, promhelp.MetricValue(t, reg, "cost_control_unpriced_token_usage_records_total",
						prometheus.Labels{"provider": "anthropic-eu", "provider_type": "anthropic", "model": "claude-sonnet-4-6"}))
				},
			},
			{
				// Budget resolves via user override, model is priced.
				name: "valid token usage with user override and cost",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					CreatedAt:      timestamppb.New(now),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					overrideGroupID := uuid.New()
					override := &database.UserAIBudgetOverride{
						UserID:           intc.InitiatorID,
						GroupID:          overrideGroupID,
						SpendLimitMicros: 1_500_000_000,
					}
					price := &database.AIModelPrice{
						Provider:   intc.Provider,
						Model:      intc.Model,
						InputPrice: sql.NullInt64{Int64: 3_000_000, Valid: true},
					}
					// No group
					expectTokenUsageCostLookups(db, intc, override, nil, nil, price)

					// input 300.
					const wantCost int64 = 300

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)

					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						// Override group wins.
						if !assert.Equal(t, uuid.NullUUID{UUID: overrideGroupID, Valid: true}, p.EffectiveGroupID, "effective group ID") ||
							!assert.Equal(t, sql.NullInt64{Int64: wantCost, Valid: true}, p.CostMicros, "cost") {
							return false
						}
						return true
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)

					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), database.IncrementUserAIDailySpendParams{
						UserID:           intc.InitiatorID,
						EffectiveGroupID: overrideGroupID,
						Day:              now.UTC().Truncate(24 * time.Hour),
						CostMicros:       wantCost,
					}).Return(database.AIUserDailySpend{}, nil)

					db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
						Return(database.GetUserAISpendSinceRow{SpendMicros: wantCost}, nil)
				},
			},
			{
				// No override or group budget, so attribution falls back to the
				// user's Everyone group.
				name: "valid token usage falls back to the Everyone group",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					CreatedAt:      timestamppb.New(now),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					everyoneID := uuid.New()
					price := &database.AIModelPrice{
						Provider:   intc.Provider,
						Model:      intc.Model,
						InputPrice: sql.NullInt64{Int64: 3_000_000, Valid: true},
					}
					expectTokenUsageCostLookups(db, intc, nil, nil, &everyoneID, price)

					// input 300.
					const wantCost int64 = 300

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)

					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						return assert.Equal(t, uuid.NullUUID{UUID: everyoneID, Valid: true}, p.EffectiveGroupID, "effective group ID") &&
							assert.Equal(t, sql.NullInt64{Int64: wantCost, Valid: true}, p.CostMicros, "cost")
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)

					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), database.IncrementUserAIDailySpendParams{
						UserID:           intc.InitiatorID,
						EffectiveGroupID: everyoneID,
						Day:              now.UTC().Truncate(24 * time.Hour),
						CostMicros:       wantCost,
					}).Return(database.AIUserDailySpend{}, nil)
				},
			},
			{
				// Model has no price row, so cost is NULL.
				name: "valid token usage with effective group and no price",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId:        uuid.NewString(),
					MsgId:                 "msg_123",
					InputTokens:           100,
					OutputTokens:          200,
					CacheReadInputTokens:  50,
					CacheWriteInputTokens: 10,
					CreatedAt:             timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					groupID := uuid.New()
					group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: 1_000_000_000}
					// Budget resolves to a group, but the model has no price row.
					// The resolved group must survive the price lookup's early
					// return on sql.ErrNoRows, while prices and cost stay NULL.
					expectTokenUsageCostLookups(db, intc, nil, group, nil, nil)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)

					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						if !assert.Equal(t, uuid.NullUUID{UUID: groupID, Valid: true}, p.EffectiveGroupID, "effective group ID") ||
							!assert.False(t, p.InputPriceMicros.Valid, "input price null") ||
							!assert.False(t, p.OutputPriceMicros.Valid, "output price null") ||
							!assert.False(t, p.CacheReadPriceMicros.Valid, "cache read price null") ||
							!assert.False(t, p.CacheWritePriceMicros.Valid, "cache write price null") ||
							!assert.False(t, p.CostMicros.Valid, "cost null") {
							return false
						}
						return true
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)

					// Spend update is skipped because cost is NULL.
					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).Times(0)
				},
				// A missing price row increments unpriced_token_usage_records_total.
				assertMetrics: func(t *testing.T, reg *prometheus.Registry) {
					require.Equal(t, 1, promhelp.CounterValue(t, reg, "cost_control_unpriced_token_usage_records_total",
						prometheus.Labels{"provider": "anthropic-eu", "provider_type": "anthropic", "model": "claude-sonnet-4-6"}))
				},
			},
			{
				// Price row exists with NULL columns, so cost is 0 (Valid).
				name: "valid token usage with effective group and NULL prices",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId:        uuid.NewString(),
					MsgId:                 "msg_123",
					InputTokens:           100,
					OutputTokens:          200,
					CacheReadInputTokens:  50,
					CacheWriteInputTokens: 10,
					CreatedAt:             timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					groupID := uuid.New()
					group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: 1_000_000_000}
					// The price row exists but every price column is NULL. Each
					// category is treated as zero for cost, so the columns are
					// recorded as NULL while cost is recorded as 0 (not NULL):
					// cost's NULL-ness tracks price row presence, not the price
					// values.
					price := &database.AIModelPrice{
						Provider:        intc.Provider,
						Model:           intc.Model,
						InputPrice:      sql.NullInt64{Valid: false},
						OutputPrice:     sql.NullInt64{Valid: false},
						CacheReadPrice:  sql.NullInt64{Valid: false},
						CacheWritePrice: sql.NullInt64{Valid: false},
					}
					expectTokenUsageCostLookups(db, intc, nil, group, nil, price)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)

					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						if !assert.Equal(t, uuid.NullUUID{UUID: groupID, Valid: true}, p.EffectiveGroupID, "effective group ID") ||
							!assert.False(t, p.InputPriceMicros.Valid, "input price null") ||
							!assert.False(t, p.OutputPriceMicros.Valid, "output price null") ||
							!assert.False(t, p.CacheReadPriceMicros.Valid, "cache read price null") ||
							!assert.False(t, p.CacheWritePriceMicros.Valid, "cache write price null") ||
							// Cost is recorded as 0 (Valid), not NULL, because the
							// price row exists.
							!assert.Equal(t, sql.NullInt64{Int64: 0, Valid: true}, p.CostMicros, "cost zero") {
							return false
						}
						return true
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)

					// Spend update is skipped because cost is 0.
					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).Times(0)
				},
			},
			{
				// Model is priced at zero, so cost is 0 (Valid).
				name: "valid token usage with effective group and zero prices",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId:        uuid.NewString(),
					MsgId:                 "msg_123",
					InputTokens:           100,
					OutputTokens:          200,
					CacheReadInputTokens:  50,
					CacheWriteInputTokens: 10,
					CreatedAt:             timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					groupID := uuid.New()
					group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: 1_000_000_000}
					// A model priced at zero is distinct from an unpriced model:
					// the price columns and cost are recorded as 0, not NULL.
					price := &database.AIModelPrice{
						Provider:        intc.Provider,
						Model:           intc.Model,
						InputPrice:      sql.NullInt64{Int64: 0, Valid: true},
						OutputPrice:     sql.NullInt64{Int64: 0, Valid: true},
						CacheReadPrice:  sql.NullInt64{Int64: 0, Valid: true},
						CacheWritePrice: sql.NullInt64{Int64: 0, Valid: true},
					}
					expectTokenUsageCostLookups(db, intc, nil, group, nil, price)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)

					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						zero := sql.NullInt64{Int64: 0, Valid: true}
						if !assert.Equal(t, uuid.NullUUID{UUID: groupID, Valid: true}, p.EffectiveGroupID, "effective group ID") ||
							!assert.Equal(t, zero, p.InputPriceMicros, "input price zero") ||
							!assert.Equal(t, zero, p.OutputPriceMicros, "output price zero") ||
							!assert.Equal(t, zero, p.CacheReadPriceMicros, "cache read price zero") ||
							!assert.Equal(t, zero, p.CacheWritePriceMicros, "cache write price zero") ||
							// Cost is 0 but recorded (Valid), not NULL.
							!assert.Equal(t, zero, p.CostMicros, "cost zero") {
							return false
						}
						return true
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)

					// Spend update is skipped because cost is 0.
					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).Times(0)
				},
			},
			{
				// No budget and no price row: attribution falls back to Everyone and cost is NULL.
				name: "valid token usage with no budget and no price",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId:        uuid.NewString(),
					MsgId:                 "msg_123",
					InputTokens:           100,
					OutputTokens:          200,
					CacheReadInputTokens:  50,
					CacheWriteInputTokens: 10,
					Metadata:              metadataProto,
					CreatedAt:             timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					// No budget configured, so attribution falls back to the
					// Everyone group. The model has no price row, so cost and
					// prices stay NULL.
					intc := newTestInterception(interceptionID)
					everyoneID := uuid.New()
					expectTokenUsageCostLookups(db, intc, nil, nil, &everyoneID, nil)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)

					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						if !assert.NotEqual(t, uuid.Nil, p.ID, "ID") ||
							!assert.Equal(t, interceptionID, p.InterceptionID, "interception ID") ||
							!assert.Equal(t, req.GetMsgId(), p.ProviderResponseID, "provider response ID") ||
							!assert.Equal(t, req.GetInputTokens(), p.InputTokens, "input tokens") ||
							!assert.Equal(t, req.GetOutputTokens(), p.OutputTokens, "output tokens") ||
							!assert.Equal(t, req.GetCacheReadInputTokens(), p.CacheReadInputTokens, "cache read input tokens") ||
							!assert.Equal(t, req.GetCacheWriteInputTokens(), p.CacheWriteInputTokens, "cache write input tokens") ||
							!assert.JSONEq(t, metadataJSON, string(p.Metadata), "metadata") ||
							!assert.WithinDuration(t, req.GetCreatedAt().AsTime(), p.CreatedAt, time.Second, "created at") ||
							!assert.Equal(t, uuid.NullUUID{UUID: everyoneID, Valid: true}, p.EffectiveGroupID, "effective group ID") ||
							!assert.False(t, p.InputPriceMicros.Valid, "input price null") ||
							!assert.False(t, p.OutputPriceMicros.Valid, "output price null") ||
							!assert.False(t, p.CacheReadPriceMicros.Valid, "cache read price null") ||
							!assert.False(t, p.CacheWritePriceMicros.Valid, "cache write price null") ||
							!assert.False(t, p.CostMicros.Valid, "cost null") {
							return false
						}
						return true
					})).Return(database.AIBridgeTokenUsage{
						ID:                    uuid.New(),
						InterceptionID:        interceptionID,
						ProviderResponseID:    req.GetMsgId(),
						InputTokens:           req.GetInputTokens(),
						OutputTokens:          req.GetOutputTokens(),
						CacheReadInputTokens:  req.GetCacheReadInputTokens(),
						CacheWriteInputTokens: req.GetCacheWriteInputTokens(),
						Metadata: pqtype.NullRawMessage{
							RawMessage: json.RawMessage(metadataJSON),
							Valid:      true,
						},
						CreatedAt: req.GetCreatedAt().AsTime(),
					}, nil)

					// Spend update is skipped because cost is NULL.
					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).Times(0)
				},
				// A missing price row increments unpriced_token_usage_records_total.
				assertMetrics: func(t *testing.T, reg *prometheus.Registry) {
					require.Equal(t, 1, promhelp.CounterValue(t, reg, "cost_control_unpriced_token_usage_records_total",
						prometheus.Labels{"provider": "anthropic-eu", "provider_type": "anthropic", "model": "claude-sonnet-4-6"}))
				},
			},
			{
				// A user with no organization has no effective group. Spend is
				// still recorded, but with a NULL group, and the daily spend
				// update is skipped.
				name: "valid token usage with no effective group",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId:        uuid.NewString(),
					MsgId:                 "msg_123",
					InputTokens:           100,
					OutputTokens:          200,
					CacheReadInputTokens:  50,
					CacheWriteInputTokens: 10,
					CreatedAt:             timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					price := &database.AIModelPrice{
						Provider:        intc.Provider,
						Model:           intc.Model,
						InputPrice:      sql.NullInt64{Int64: 3_000_000, Valid: true},
						OutputPrice:     sql.NullInt64{Int64: 6_000_000, Valid: true},
						CacheReadPrice:  sql.NullInt64{Int64: 300_000, Valid: true},
						CacheWritePrice: sql.NullInt64{Int64: 4_000_000, Valid: true},
					}
					// Every resolution lookup misses, including the Everyone
					// fallback, so the group stays NULL while cost is computed.
					expectTokenUsageCostLookups(db, intc, nil, nil, nil, price)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)

					// input 300 + output 1200 + cache read 15 + cache write 40.
					const wantCost int64 = 1555

					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						if !assert.False(t, p.EffectiveGroupID.Valid, "effective group ID null") ||
							!assert.Equal(t, sql.NullInt64{Int64: wantCost, Valid: true}, p.CostMicros, "cost") {
							return false
						}
						return true
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)

					// Spend update is skipped because the effective group is NULL.
					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).Times(0)
				},
			},
			{
				// An azure provider has the openai upstream wire format but bills at
				// its own rates.
				name: "openai wire format priced as azure",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					intc.Provider = "openai"
					intc.ProviderName = "azure-prod"
					intc.Model = "gpt-5-mini"
					expectBudgetLookups(db, intc)

					db.EXPECT().GetAIProviderByName(gomock.Any(), intc.ProviderName).
						Return(database.AIProvider{Name: intc.ProviderName, Type: database.AIProviderTypeAzure}, nil)
					db.EXPECT().GetAIModelPriceByProviderModel(gomock.Any(), database.GetAIModelPriceByProviderModelParams{
						Provider: string(database.AIProviderTypeAzure),
						Model:    intc.Model,
					}).Return(database.AIModelPrice{
						Provider:   string(database.AIProviderTypeAzure),
						Model:      intc.Model,
						InputPrice: sql.NullInt64{Int64: 3_000_000, Valid: true},
					}, nil)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)
					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						return assert.Equal(t, sql.NullInt64{Int64: 3_000_000, Valid: true}, p.InputPriceMicros, "input price") &&
							assert.Equal(t, sql.NullInt64{Int64: 300, Valid: true}, p.CostMicros, "cost")
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)
					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).
						Return(database.AIUserDailySpend{}, nil)
				},
			},
			{
				// A bedrock provider has the anthropic upstream wire format but bills
				// at its own rates.
				name: "anthropic wire format priced as bedrock",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					intc.ProviderName = "bedrock-eu"
					expectBudgetLookups(db, intc)

					db.EXPECT().GetAIProviderByName(gomock.Any(), intc.ProviderName).
						Return(database.AIProvider{Name: intc.ProviderName, Type: database.AIProviderTypeBedrock}, nil)
					db.EXPECT().GetAIModelPriceByProviderModel(gomock.Any(), database.GetAIModelPriceByProviderModelParams{
						Provider: string(database.AIProviderTypeBedrock),
						Model:    intc.Model,
					}).Return(database.AIModelPrice{
						Provider:   string(database.AIProviderTypeBedrock),
						Model:      intc.Model,
						InputPrice: sql.NullInt64{Int64: 3_000_000, Valid: true},
					}, nil)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)
					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						return assert.Equal(t, sql.NullInt64{Int64: 3_000_000, Valid: true}, p.InputPriceMicros, "input price") &&
							assert.Equal(t, sql.NullInt64{Int64: 300, Valid: true}, p.CostMicros, "cost")
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)
					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).
						Return(database.AIUserDailySpend{}, nil)
				},
			},
			{
				name: "unresolved provider is unpriced",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					expectBudgetLookups(db, intc)

					db.EXPECT().GetAIProviderByName(gomock.Any(), intc.ProviderName).
						Return(database.AIProvider{}, sql.ErrNoRows)
					// Without a provider there is nothing to key the price on.
					db.EXPECT().GetAIModelPriceByProviderModel(gomock.Any(), gomock.Any()).Times(0)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)
					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						return assert.False(t, p.InputPriceMicros.Valid, "input price null") &&
							assert.False(t, p.CostMicros.Valid, "cost null")
					})).Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)
					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).Times(0)
				},
				// The metric names the provider that failed to resolve.
				assertMetrics: func(t *testing.T, reg *prometheus.Registry) {
					require.Equal(t, 1, promhelp.CounterValue(t, reg, "cost_control_unpriced_token_usage_records_total",
						prometheus.Labels{"provider": "anthropic-eu", "provider_type": "unknown", "model": "claude-sonnet-4-6"}))
				},
			},
			{
				name: "invalid interception ID",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: "not-a-uuid",
					MsgId:          "msg_123",
					InputTokens:    100,
					OutputTokens:   200,
					CreatedAt:      timestamppb.Now(),
				},
				expectedErr: "failed to parse interception_id",
			},
			{
				name: "interception lookup error",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					OutputTokens:   200,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					// An unexpected interception lookup error fails the record;
					// no token usage is inserted.
					db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), interceptionID).
						Return(database.AIBridgeInterception{}, sql.ErrConnDone)
				},
				expectedErr: "get interception",
			},
			{
				name: "provider lookup error",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					// An unexpected provider lookup error (not sql.ErrNoRows) fails
					// the record.
					intc := newTestInterception(interceptionID)
					expectBudgetLookups(db, intc)
					db.EXPECT().GetAIProviderByName(gomock.Any(), intc.ProviderName).
						Return(database.AIProvider{}, sql.ErrConnDone)
				},
				expectedErr: "get configured provider",
			},
			{
				name: "price lookup error",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					OutputTokens:   200,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					// An unexpected price lookup error (not sql.ErrNoRows) fails
					// the record.
					intc := newTestInterception(interceptionID)
					db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), interceptionID).Return(intc, nil)
					db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), intc.InitiatorID).
						Return(database.UserAIBudgetOverride{}, sql.ErrNoRows)
					db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), intc.InitiatorID).
						Return(database.GetHighestGroupAIBudgetByUserRow{}, sql.ErrNoRows)
					db.EXPECT().GetUserEveryoneFallbackGroup(gomock.Any(), intc.InitiatorID).
						Return(uuid.New(), nil)
					db.EXPECT().GetAIProviderByName(gomock.Any(), intc.ProviderName).
						Return(database.AIProvider{Name: intc.ProviderName, Type: database.AIProviderTypeAnthropic}, nil)
					db.EXPECT().GetAIModelPriceByProviderModel(gomock.Any(), gomock.Any()).
						Return(database.AIModelPrice{}, sql.ErrConnDone)
				},
				expectedErr: "resolve token usage cost",
			},
			{
				name: "insert token usage error",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					OutputTokens:   200,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					everyoneID := uuid.New()
					expectTokenUsageCostLookups(db, newTestInterception(interceptionID), nil, nil, &everyoneID, nil)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)
					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).Return(database.AIBridgeTokenUsage{}, sql.ErrConnDone)
				},
				expectedErr: "insert token usage",
			},
			{
				name: "increment user daily spend error",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					intc := newTestInterception(interceptionID)
					group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: uuid.New(), SpendLimitMicros: 1_000_000_000}
					price := &database.AIModelPrice{
						Provider:   intc.Provider,
						Model:      intc.Model,
						InputPrice: sql.NullInt64{Int64: 3_000_000, Valid: true},
					}
					expectTokenUsageCostLookups(db, intc, nil, group, nil, price)

					db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
						func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
					)
					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).
						Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: interceptionID}, nil)
					db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).
						Return(database.AIUserDailySpend{}, sql.ErrConnDone)
				},
				expectedErr: "increment user daily spend",
			},
		},
	)
}

// TestRecordTokenUsageAuthorized exercises RecordTokenUsage end-to-end against a
// real database through the dbauthz layer as subjectAibridged. This catches missing
// RBAC grants on the aibridged subject and verifies the cost columns round-trip
// to storage along with the daily spend row.
func TestRecordTokenUsageAuthorized(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)

	rawDB, _ := dbtestutil.NewDB(t)
	authzDB := dbauthz.New(rawDB, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())

	// Seed prerequisites via the raw (unauthorized) store. The user belongs to a
	// group with a budget, so the effective group resolves to that group.
	org := dbgen.Organization(t, rawDB, database.Organization{})
	user := dbgen.User(t, rawDB, database.User{})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})
	group := dbgen.Group(t, rawDB, database.Group{OrganizationID: org.ID})
	dbgen.GroupMember(t, rawDB, database.GroupMemberTable{UserID: user.ID, GroupID: group.ID})

	_, err := rawDB.UpsertGroupAIBudget(ctx, database.UpsertGroupAIBudgetParams{
		GroupID:          group.ID,
		SpendLimitMicros: 1_000_000_000,
	})
	require.NoError(t, err, "upsert group AI budget")

	const provider, model = "anthropic", "claude-sonnet-4-6"
	priceSeed, err := json.Marshal([]map[string]any{{
		"provider":          provider,
		"model":             model,
		"input_price":       3_000_000,
		"output_price":      6_000_000,
		"cache_read_price":  300_000,
		"cache_write_price": 4_000_000,
	}})
	require.NoError(t, err)
	require.NoError(t, rawDB.UpsertAIModelPrices(ctx, priceSeed), "seed model prices")

	// The interception's provider name resolves to this provider, whose type keys
	// the price lookup.
	aiProvider := dbgen.AIProvider(t, rawDB, database.AIProvider{
		Name: "anthropic-eu",
		Type: database.AIProviderTypeAnthropic,
	})

	intc := dbgen.AIBridgeInterception(t, rawDB, database.InsertAIBridgeInterceptionParams{
		InitiatorID:  user.ID,
		Provider:     provider,
		ProviderName: aiProvider.Name,
		Model:        model,
	}, nil)

	// Use fixed dates to keep the test deterministic.
	now := time.Date(2026, 6, 25, 14, 30, 0, 0, time.UTC)

	// The server runs every store call as subjectAibridged via the authzDB.
	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         authzDB,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Experiments:   requiredExperiments,
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	_, err = srv.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
		InterceptionId:        intc.ID.String(),
		MsgId:                 "msg_e2e",
		InputTokens:           100,
		OutputTokens:          200,
		CacheReadInputTokens:  50,
		CacheWriteInputTokens: 10,
		CreatedAt:             timestamppb.New(now),
	})
	require.NoError(t, err, "record token usage")

	// Read the persisted row back via the raw store and verify the snapshot.
	tokenUsages, err := rawDB.GetAIBridgeTokenUsagesByInterceptionID(ctx, intc.ID)
	require.NoError(t, err)
	require.Len(t, tokenUsages, 1)
	tokenUsage := tokenUsages[0]

	require.Equal(t, uuid.NullUUID{UUID: group.ID, Valid: true}, tokenUsage.EffectiveGroupID, "effective group")
	require.Equal(t, sql.NullInt64{Int64: 3_000_000, Valid: true}, tokenUsage.InputPriceMicros, "input price")
	require.Equal(t, sql.NullInt64{Int64: 6_000_000, Valid: true}, tokenUsage.OutputPriceMicros, "output price")
	require.Equal(t, sql.NullInt64{Int64: 300_000, Valid: true}, tokenUsage.CacheReadPriceMicros, "cache read price")
	require.Equal(t, sql.NullInt64{Int64: 4_000_000, Valid: true}, tokenUsage.CacheWritePriceMicros, "cache write price")
	// input 300 + output 1200 + cache read 15 + cache write 40.
	const wantCost int64 = 1555
	require.Equal(t, sql.NullInt64{Int64: wantCost, Valid: true}, tokenUsage.CostMicros, "cost")

	// The daily spend row was incremented for (user, group, today) by the same cost.
	today := now.UTC().Truncate(24 * time.Hour)
	spend, err := rawDB.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
		UserID:           user.ID,
		EffectiveGroupID: group.ID,
		PeriodStart:      today,
	})
	require.NoError(t, err, "get user AI spend since")
	require.Equal(t, user.ID, spend.UserID, "user ID")
	require.Equal(t, group.ID, spend.EffectiveGroupID, "effective group ID")
	require.True(t, today.Equal(spend.PeriodStart), "period start: want %s, got %s", today, spend.PeriodStart)
	require.Equal(t, wantCost, spend.SpendMicros, "spend micros")
}

// TestRecordTokenUsageProviderResolution covers provider resolution against a real
// database through dbauthz, where the live-row filter and name reuse apply.
func TestRecordTokenUsageProviderResolution(t *testing.T) {
	t.Parallel()

	const claudeModel, gptModel = "claude-sonnet-4-6", "gpt-5-mini"
	const anthropicInputPrice, bedrockInputPrice, openaiInputPrice, azureInputPrice int64 = 2_000_000, 3_000_000, 4_000_000, 5_000_000

	setupCtx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)

	rawDB, _ := dbtestutil.NewDB(t)
	authzDB := dbauthz.New(rawDB, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())

	user := dbgen.User(t, rawDB, database.User{})

	// Prices differ per provider type so the asserted cost identifies which type resolved.
	priceSeed, err := json.Marshal([]map[string]any{
		{"provider": string(database.AIProviderTypeAnthropic), "model": claudeModel, "input_price": anthropicInputPrice},
		{"provider": string(database.AIProviderTypeBedrock), "model": claudeModel, "input_price": bedrockInputPrice},
		{"provider": string(database.AIProviderTypeOpenai), "model": gptModel, "input_price": openaiInputPrice},
		{"provider": string(database.AIProviderTypeAzure), "model": gptModel, "input_price": azureInputPrice},
	})
	require.NoError(t, err)
	require.NoError(t, rawDB.UpsertAIModelPrices(setupCtx, priceSeed), "seed model prices")

	srv, err := aibridgedserver.NewServer(setupCtx, aibridgedserver.Options{
		Store:         authzDB,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Experiments:   requiredExperiments,
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	cases := []struct {
		name string
		// wireProvider is the upstream wire format recorded on the interception.
		wireProvider string
		// providerName is the provider instance name recorded on the interception.
		providerName string
		// providerType is the configured provider type of the live provider.
		providerType database.AIProviderType
		model        string
		// setupProvider creates and deletes the case's providers.
		setupProvider  func(t *testing.T, ctx context.Context, providerName string, providerType database.AIProviderType)
		wantInputPrice sql.NullInt64
		wantCost       sql.NullInt64
	}{
		{
			// The common configuration, where the configured provider type matches
			// the upstream wire format.
			name:         "provider named after its type",
			wireProvider: "anthropic",
			providerName: "anthropic",
			providerType: database.AIProviderTypeAnthropic,
			model:        claudeModel,
			// One live anthropic provider.
			setupProvider: func(t *testing.T, _ context.Context, providerName string, providerType database.AIProviderType) {
				dbgen.AIProvider(t, rawDB, database.AIProvider{Name: providerName, Type: providerType})
			},
			wantInputPrice: sql.NullInt64{Int64: anthropicInputPrice, Valid: true},
			// 100 input tokens at the anthropic input price: $0.0002.
			wantCost: sql.NullInt64{Int64: 200, Valid: true},
		},
		{
			name:         "priced by configured provider type",
			wireProvider: "anthropic",
			providerName: "bedrock-eu",
			providerType: database.AIProviderTypeBedrock,
			model:        claudeModel,
			// One live bedrock provider.
			setupProvider: func(t *testing.T, _ context.Context, providerName string, providerType database.AIProviderType) {
				dbgen.AIProvider(t, rawDB, database.AIProvider{Name: providerName, Type: providerType})
			},
			wantInputPrice: sql.NullInt64{Int64: bedrockInputPrice, Valid: true},
			// 100 input tokens at the bedrock input price: $0.0003.
			wantCost: sql.NullInt64{Int64: 300, Valid: true},
		},
		{
			name:         "deleted provider is unpriced",
			wireProvider: "anthropic",
			providerName: "bedrock-deleted",
			providerType: database.AIProviderTypeBedrock,
			model:        claudeModel,
			// One bedrock provider, deleted before the usage is recorded.
			setupProvider: func(t *testing.T, ctx context.Context, providerName string, providerType database.AIProviderType) {
				provider := dbgen.AIProvider(t, rawDB, database.AIProvider{Name: providerName, Type: providerType})
				require.NoError(t, rawDB.DeleteAIProviderByID(ctx, provider.ID), "delete provider")
			},
			wantInputPrice: sql.NullInt64{Valid: false},
			wantCost:       sql.NullInt64{Valid: false},
		},
		{
			// Names are unique only among live providers, so a deleted name can be
			// reused by a provider of a different configured provider type.
			name:         "reused name resolves to the live provider",
			wireProvider: "openai",
			providerName: "reused-name",
			providerType: database.AIProviderTypeAzure,
			model:        gptModel,
			// A deleted openai provider and a live azure provider sharing the name.
			setupProvider: func(t *testing.T, ctx context.Context, providerName string, providerType database.AIProviderType) {
				deleted := dbgen.AIProvider(t, rawDB, database.AIProvider{Name: providerName, Type: database.AIProviderTypeOpenai})
				require.NoError(t, rawDB.DeleteAIProviderByID(ctx, deleted.ID), "delete provider")
				dbgen.AIProvider(t, rawDB, database.AIProvider{Name: providerName, Type: providerType})
			},
			wantInputPrice: sql.NullInt64{Int64: azureInputPrice, Valid: true},
			// 100 input tokens at the azure input price: $0.0005.
			wantCost: sql.NullInt64{Int64: 500, Valid: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			tc.setupProvider(t, ctx, tc.providerName, tc.providerType)

			intc := dbgen.AIBridgeInterception(t, rawDB, database.InsertAIBridgeInterceptionParams{
				InitiatorID:  user.ID,
				Provider:     tc.wireProvider,
				ProviderName: tc.providerName,
				Model:        tc.model,
			}, nil)

			_, err := srv.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
				InterceptionId: intc.ID.String(),
				MsgId:          "msg_e2e",
				InputTokens:    100,
				CreatedAt:      timestamppb.Now(),
			})
			require.NoError(t, err, "record token usage")

			tokenUsages, err := rawDB.GetAIBridgeTokenUsagesByInterceptionID(ctx, intc.ID)
			require.NoError(t, err)
			require.Len(t, tokenUsages, 1)
			require.Equal(t, tc.wantInputPrice, tokenUsages[0].InputPriceMicros, "input price")
			require.Equal(t, tc.wantCost, tokenUsages[0].CostMicros, "cost")
		})
	}
}

// TestRecordTokenUsageBudgetNotifications verifies that recording token usage
// enqueues the right budget notifications: the warning template when spend
// crosses the warning threshold, the limit-reached template at 100%, both when
// a single interception crosses both, and nothing when no threshold is
// freshly crossed.
func TestRecordTokenUsageBudgetNotifications(t *testing.T) {
	t.Parallel()

	const (
		dollar          int64 = 1_000_000    // micros per USD dollar
		spendLimit      int64 = 100 * dollar // $100 limit (100% threshold)
		warnAt          int64 = 85 * dollar  // $85 (85% of the limit)
		inputPrice      int64 = dollar       // $1 per million tokens
		tokensPerDollar int64 = 1_000_000    // 1,000,000 tokens = $1 at the price above
	)
	now := time.Date(2026, 6, 25, 14, 30, 0, 0, time.UTC)

	// Each case sets its cost via inputTokens (tokensPerDollar tokens = $1).
	// newSpend is the post-increment period total; the code derives the
	// pre-increment total as newSpend - cost and fires a threshold only when the
	// pre-increment total is below the threshold and the post-increment total is
	// at or above it.
	price := &database.AIModelPrice{
		InputPrice: sql.NullInt64{Int64: inputPrice, Valid: true},
	}

	// Threshold percentage label expected for each template.
	wantThreshold := map[uuid.UUID]string{
		notifications.TemplateAIBudgetWarningUser:      "85",
		notifications.TemplateAIBudgetLimitReachedUser: "100",
	}

	testCases := []struct {
		name          string
		inputTokens   int64
		newSpend      int64 // post-increment period total
		wantTemplates []uuid.UUID
	}{
		{
			name: "crosses warning threshold",
			// pre = $84.50 (< $85), post = $85.50 (>= $85) -> warning.
			inputTokens:   tokensPerDollar,
			newSpend:      warnAt + dollar/2,
			wantTemplates: []uuid.UUID{notifications.TemplateAIBudgetWarningUser},
		},
		{
			name: "crosses warning threshold exactly",
			// pre = $84 (< $85), post = $85 (>= $85) -> warning.
			inputTokens:   tokensPerDollar,
			newSpend:      warnAt,
			wantTemplates: []uuid.UUID{notifications.TemplateAIBudgetWarningUser},
		},
		{
			name: "stays below warning threshold",
			// post = $84.50 (< $85) -> no crossing.
			inputTokens:   tokensPerDollar,
			newSpend:      warnAt - dollar/2,
			wantTemplates: nil,
		},
		{
			name: "already at warning threshold",
			// pre = $85 (not < $85), post = $86 -> no fresh crossing.
			inputTokens:   tokensPerDollar,
			newSpend:      warnAt + dollar,
			wantTemplates: nil,
		},
		{
			name: "already above warning threshold",
			// pre = $89 (>= $85, < $100), post = $90 -> no crossing.
			inputTokens:   tokensPerDollar,
			newSpend:      warnAt + 5*dollar,
			wantTemplates: nil,
		},
		{
			name: "crosses limit",
			// pre = $99.50 (>= $85, so no warning; < $100), post = $100.50 (>= $100) -> limit.
			inputTokens:   tokensPerDollar,
			newSpend:      spendLimit + dollar/2,
			wantTemplates: []uuid.UUID{notifications.TemplateAIBudgetLimitReachedUser},
		},
		{
			name: "crosses warning and limit in one interception",
			// pre = $80 (< $85), post = $100 (>= $85 and >= $100) -> warning + limit.
			inputTokens: 20 * tokensPerDollar,
			newSpend:    spendLimit,
			wantTemplates: []uuid.UUID{
				notifications.TemplateAIBudgetWarningUser,
				notifications.TemplateAIBudgetLimitReachedUser,
			},
		},
		{
			name: "already above limit",
			// pre = $109 (>= $100), post = $110 -> no fresh crossing.
			inputTokens:   tokensPerDollar,
			newSpend:      spendLimit + 10*dollar,
			wantTemplates: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			enq := &notificationstest.FakeEnqueuer{}

			intc := newTestInterception(uuid.New())
			groupID := uuid.New()
			group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: spendLimit}

			expectTokenUsageCostLookups(db, intc, nil, group, nil, price)

			db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
				func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
			)
			db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).
				Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: intc.ID}, nil)
			db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).
				Return(database.AIUserDailySpend{}, nil)
			db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
				Return(database.GetUserAISpendSinceRow{SpendMicros: tc.newSpend}, nil)
			// The group and user are resolved once per interception that
			// notifies, regardless of how many thresholds it crosses.
			if len(tc.wantTemplates) > 0 {
				db.EXPECT().GetGroupByID(gomock.Any(), groupID).
					Return(database.Group{ID: groupID, Name: "Engineering"}, nil)
				db.EXPECT().GetUserByID(gomock.Any(), intc.InitiatorID).
					Return(database.User{ID: intc.InitiatorID, Username: "bob"}, nil)
				// No admins configured, so only the user is notified.
				db.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(nil, nil)
			}

			ctx := testutil.Context(t, testutil.WaitLong)
			srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
				Store:         db,
				AISeatTracker: agplaiseats.Noop{},
				AccessURL:     "/",
				GatewayCfg:    codersdk.AIBridgeConfig{},
				Experiments:   requiredExperiments,
				Enqueuer:      enq,
				Logger:        testutil.Logger(t),
				Clock:         quartz.NewReal(),
			})
			require.NoError(t, err)

			_, err = srv.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
				InterceptionId: intc.ID.String(),
				MsgId:          "msg_123",
				InputTokens:    tc.inputTokens,
				CreatedAt:      timestamppb.New(now),
			})
			require.NoError(t, err)

			require.Len(t, enq.Sent(), len(tc.wantTemplates), "unexpected number of notifications")
			for _, tmpl := range tc.wantTemplates {
				sent := enq.Sent(notificationstest.WithTemplateID(tmpl))
				require.Len(t, sent, 1, "expected one notification for template %s", tmpl)
				require.Equal(t, intc.InitiatorID, sent[0].UserID)
				require.Equal(t, wantThreshold[tmpl], sent[0].Labels["threshold"])
				require.Equal(t, "$100.00", sent[0].Labels["limit"])
				require.Equal(t, "Engineering", sent[0].Labels["effective_group_name"])
				// The interception is recorded at 2026-06-25, so its budget
				// period runs June 1 - July 1, 2026.
				require.Equal(t, "June 1, 2026", sent[0].Labels["period_start"])
				require.Equal(t, "July 1, 2026", sent[0].Labels["period_end"])
			}
		})
	}
}

// TestRecordTokenUsageBudgetNotificationAcrossPeriodBoundary verifies that an
// interception created in one budget period but processed after the period has
// rolled over is still evaluated against the period it belongs to, so a genuine
// threshold crossing is detected rather than lost across the boundary.
func TestRecordTokenUsageBudgetNotificationAcrossPeriodBoundary(t *testing.T) {
	t.Parallel()

	const (
		dollar          int64 = 1_000_000    // micros per USD dollar
		spendLimit      int64 = 100 * dollar // $100 limit (100% threshold)
		warnAt          int64 = 85 * dollar  // $85 (85% of the limit)
		inputPrice      int64 = dollar       // $1 per million tokens
		tokensPerDollar int64 = 1_000_000    // 1,000,000 tokens = $1 at the price above
	)

	// The interception was created in the final second of January but is
	// processed just after the rollover into February. The spend is bucketed
	// into January, so detection must sum against January's period.
	createdAt := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
	processedAt := time.Date(2026, 2, 1, 0, 0, 1, 0, time.UTC)
	januaryStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	enq := &notificationstest.FakeEnqueuer{}

	intc := newTestInterception(uuid.New())
	groupID := uuid.New()
	group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: spendLimit}
	price := &database.AIModelPrice{InputPrice: sql.NullInt64{Int64: inputPrice, Valid: true}}

	expectTokenUsageCostLookups(db, intc, nil, group, nil, price)

	db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
		func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
	)
	db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).
		Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: intc.ID}, nil)
	db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).
		Return(database.AIUserDailySpend{}, nil)

	// The spend query must run against the period the interception belongs to
	// (January), not the period it was processed in (February).
	var gotPeriodStart time.Time
	db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p database.GetUserAISpendSinceParams) (database.GetUserAISpendSinceRow, error) {
			gotPeriodStart = p.PeriodStart
			return database.GetUserAISpendSinceRow{SpendMicros: warnAt}, nil
		})
	db.EXPECT().GetGroupByID(gomock.Any(), groupID).
		Return(database.Group{ID: groupID, Name: "Engineering"}, nil)
	db.EXPECT().GetUserByID(gomock.Any(), intc.InitiatorID).
		Return(database.User{ID: intc.InitiatorID, Username: "bob"}, nil)
	// No admins configured, so only the user is notified.
	db.EXPECT().GetUsers(gomock.Any(), gomock.Any()).Return(nil, nil)

	clock := quartz.NewMock(t)
	clock.Set(processedAt)

	ctx := testutil.Context(t, testutil.WaitLong)
	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         db,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Experiments:   requiredExperiments,
		Enqueuer:      enq,
		Logger:        testutil.Logger(t),
		Clock:         clock,
	})
	require.NoError(t, err)

	_, err = srv.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
		InterceptionId: intc.ID.String(),
		MsgId:          "msg_boundary",
		InputTokens:    tokensPerDollar,
		CreatedAt:      timestamppb.New(createdAt),
	})
	require.NoError(t, err)

	require.Equal(t, januaryStart, gotPeriodStart,
		"spend must be summed against the period the interception belongs to, not the processing period")

	sent := enq.Sent(notificationstest.WithTemplateID(notifications.TemplateAIBudgetWarningUser))
	require.Len(t, sent, 1, "expected the crossing to be detected against the interception's period")
}

// TestRecordTokenUsageBudgetNotificationBestEffort verifies that a failure while
// detecting or sending a budget notification is swallowed: the token usage and
// spend are still recorded (RecordTokenUsage returns no error) and no
// notification is enqueued. This guards the best-effort contract, e.g. that a
// detection error is not propagated out of the transaction (which would roll
// back the committed spend).
func TestRecordTokenUsageBudgetNotificationBestEffort(t *testing.T) {
	t.Parallel()

	const (
		dollar          int64 = 1_000_000
		spendLimit      int64 = 100 * dollar
		warnAt          int64 = 85 * dollar
		inputPrice      int64 = dollar
		tokensPerDollar int64 = 1_000_000
	)
	now := time.Date(2026, 6, 25, 14, 30, 0, 0, time.UTC)
	price := &database.AIModelPrice{InputPrice: sql.NullInt64{Int64: inputPrice, Valid: true}}

	testCases := []struct {
		name string
		// spendSinceErr fails detection (the read inside the transaction);
		// groupLookupErr fails the notification after a crossing is detected.
		spendSinceErr  error
		groupLookupErr error
	}{
		{name: "detection read fails", spendSinceErr: sql.ErrConnDone},
		{name: "group lookup fails", groupLookupErr: sql.ErrConnDone},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			enq := &notificationstest.FakeEnqueuer{}

			intc := newTestInterception(uuid.New())
			groupID := uuid.New()
			group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: spendLimit}

			expectTokenUsageCostLookups(db, intc, nil, group, nil, price)

			db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
				func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
			)
			// The token usage and spend are recorded before detection runs.
			db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).
				Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: intc.ID}, nil)
			db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).
				Return(database.AIUserDailySpend{}, nil)

			switch {
			case tc.spendSinceErr != nil:
				// Detection fails; the group is never looked up.
				db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
					Return(database.GetUserAISpendSinceRow{}, tc.spendSinceErr)
			case tc.groupLookupErr != nil:
				// A crossing is detected ($84 -> $85), but resolving the group
				// for the notification fails.
				db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
					Return(database.GetUserAISpendSinceRow{SpendMicros: warnAt}, nil)
				db.EXPECT().GetGroupByID(gomock.Any(), groupID).
					Return(database.Group{}, tc.groupLookupErr)
			default:
				t.Fatal("test case must set spendSinceErr or groupLookupErr")
			}

			ctx := testutil.Context(t, testutil.WaitLong)
			srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
				Store:         db,
				AISeatTracker: agplaiseats.Noop{},
				AccessURL:     "/",
				GatewayCfg:    codersdk.AIBridgeConfig{},
				Experiments:   requiredExperiments,
				Enqueuer:      enq,
				// The detect/notify failure is logged; ignore it here since
				// triggering it is the point of the test.
				Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
				Clock:  quartz.NewReal(),
			})
			require.NoError(t, err)

			// The failure must not surface as an error from RecordTokenUsage.
			_, err = srv.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
				InterceptionId: intc.ID.String(),
				MsgId:          "msg_123",
				InputTokens:    tokensPerDollar,
				CreatedAt:      timestamppb.New(now),
			})
			require.NoError(t, err)
			require.Empty(t, enq.Sent(), "no notification should be enqueued when detection or lookup fails")
		})
	}
}

// TestRecordTokenUsageBudgetNotificationZeroLimit verifies that a zero spend
// limit (used to block a group entirely) produces no budget notification: there
// is no meaningful threshold to cross, and such users are already blocked by
// pre-request enforcement. The token usage is still recorded.
func TestRecordTokenUsageBudgetNotificationZeroLimit(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	enq := &notificationstest.FakeEnqueuer{}

	intc := newTestInterception(uuid.New())
	groupID := uuid.New()
	// A zero limit blocks the group; there is no threshold to cross.
	group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: 0}
	price := &database.AIModelPrice{InputPrice: sql.NullInt64{Int64: 1_000_000, Valid: true}}

	expectTokenUsageCostLookups(db, intc, nil, group, nil, price)

	db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
		func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
	)
	// The spend is still recorded; detection then short-circuits on the zero
	// limit without reading spend or looking up the group.
	db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).
		Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: intc.ID}, nil)
	db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).
		Return(database.AIUserDailySpend{}, nil)

	ctx := testutil.Context(t, testutil.WaitLong)
	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         db,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Experiments:   requiredExperiments,
		Enqueuer:      enq,
		Logger:        testutil.Logger(t),
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	_, err = srv.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
		InterceptionId: intc.ID.String(),
		MsgId:          "msg_123",
		InputTokens:    1_000_000, // $1 at $1 per million tokens
		CreatedAt:      timestamppb.New(time.Date(2026, 6, 25, 14, 30, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	require.Empty(t, enq.Sent(), "a zero spend limit must not produce a notification")
}

// TestRecordTokenUsageBudgetAdminNotification verifies that crossing a budget
// threshold also notifies deployment owners and user admins, and that the
// affected user is not double-notified as an admin.
func TestRecordTokenUsageBudgetAdminNotification(t *testing.T) {
	t.Parallel()

	const (
		dollar          int64 = 1_000_000    // micros per USD dollar
		spendLimit      int64 = 100 * dollar // $100 limit (100% threshold)
		warnAt          int64 = 85 * dollar  // $85 (85% of the limit)
		inputPrice      int64 = dollar       // $1 per million tokens
		tokensPerDollar int64 = 1_000_000    // 1,000,000 tokens = $1 at the price above
	)
	now := time.Date(2026, 6, 25, 14, 30, 0, 0, time.UTC)

	cases := []struct {
		name              string
		useOverride       bool
		inputTokens       int64
		newSpend          int64
		wantThreshold     string
		wantLimitSource   string
		wantUserTemplate  uuid.UUID
		wantAdminTemplate uuid.UUID
	}{
		{
			// A $1 interception takes spend to warnAt ($85): pre ($84) < warnAt <= post.
			name:              "warning",
			inputTokens:       tokensPerDollar,
			newSpend:          warnAt,
			wantThreshold:     "85",
			wantLimitSource:   string(codersdk.AIBudgetLimitSourceGroup),
			wantUserTemplate:  notifications.TemplateAIBudgetWarningUser,
			wantAdminTemplate: notifications.TemplateAIBudgetWarningAdmin,
		},
		{
			// A $5 interception takes spend from $95 to the $100 limit, so the limit threshold is crossed.
			name:              "limit reached",
			inputTokens:       5 * tokensPerDollar,
			newSpend:          spendLimit,
			wantThreshold:     "100",
			wantLimitSource:   string(codersdk.AIBudgetLimitSourceGroup),
			wantUserTemplate:  notifications.TemplateAIBudgetLimitReachedUser,
			wantAdminTemplate: notifications.TemplateAIBudgetLimitReachedAdmin,
		},
		{
			// A per-user override supplies the limit, so limit_source is user_override.
			name:              "warning, per-user override",
			useOverride:       true,
			inputTokens:       tokensPerDollar,
			newSpend:          warnAt,
			wantThreshold:     "85",
			wantLimitSource:   string(codersdk.AIBudgetLimitSourceUserOverride),
			wantUserTemplate:  notifications.TemplateAIBudgetWarningUser,
			wantAdminTemplate: notifications.TemplateAIBudgetWarningAdmin,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			price := &database.AIModelPrice{
				InputPrice: sql.NullInt64{Int64: inputPrice, Valid: true},
			}

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			enq := &notificationstest.FakeEnqueuer{}

			intc := newTestInterception(uuid.New())
			groupID := uuid.New()

			admin := database.GetUsersRow{ID: uuid.New(), Username: "admin1"}
			// The affected user is also an admin; they must not receive the admin copy.
			selfAdmin := database.GetUsersRow{ID: intc.InitiatorID, Username: "bob"}

			// Whether the limit comes from a group budget or a per-user override, the
			// spend is attributed to the same effective group, so the group lookup and
			// notifications are identical apart from the limit_source label.
			if tc.useOverride {
				override := &database.UserAIBudgetOverride{GroupID: groupID, SpendLimitMicros: spendLimit}
				expectTokenUsageCostLookups(db, intc, override, nil, nil, price)
			} else {
				group := &database.GetHighestGroupAIBudgetByUserRow{GroupID: groupID, SpendLimitMicros: spendLimit}
				expectTokenUsageCostLookups(db, intc, nil, group, nil, price)
			}
			db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
				func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
			)
			db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).
				Return(database.AIBridgeTokenUsage{ID: uuid.New(), InterceptionID: intc.ID}, nil)
			db.EXPECT().IncrementUserAIDailySpend(gomock.Any(), gomock.Any()).
				Return(database.AIUserDailySpend{}, nil)
			db.EXPECT().GetUserAISpendSince(gomock.Any(), gomock.Any()).
				Return(database.GetUserAISpendSinceRow{SpendMicros: tc.newSpend}, nil)
			db.EXPECT().GetGroupByID(gomock.Any(), groupID).
				Return(database.Group{ID: groupID, Name: "Engineering"}, nil)
			db.EXPECT().GetUserByID(gomock.Any(), intc.InitiatorID).
				Return(database.User{ID: intc.InitiatorID, Username: "bob"}, nil)
			db.EXPECT().GetUsers(gomock.Any(), database.GetUsersParams{
				RbacRole: []string{codersdk.RoleOwner, codersdk.RoleUserAdmin},
			}).Return([]database.GetUsersRow{admin, selfAdmin}, nil)

			ctx := testutil.Context(t, testutil.WaitLong)
			srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
				Store:         db,
				AISeatTracker: agplaiseats.Noop{},
				AccessURL:     "/",
				GatewayCfg:    codersdk.AIBridgeConfig{},
				Experiments:   requiredExperiments,
				Enqueuer:      enq,
				Logger:        testutil.Logger(t),
				Clock:         quartz.NewReal(),
			})
			require.NoError(t, err)

			_, err = srv.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
				InterceptionId: intc.ID.String(),
				MsgId:          "msg_123",
				InputTokens:    tc.inputTokens,
				CreatedAt:      timestamppb.New(now),
			})
			require.NoError(t, err)

			// The user who crossed the threshold gets the user-facing notification.
			userSent := enq.Sent(notificationstest.WithTemplateID(tc.wantUserTemplate))
			require.Len(t, userSent, 1)
			require.Equal(t, intc.InitiatorID, userSent[0].UserID)

			// The admin (but not the affected user, who is also an admin) gets the
			// admin notification naming the affected user.
			adminSent := enq.Sent(notificationstest.WithTemplateID(tc.wantAdminTemplate))
			require.Len(t, adminSent, 1)
			require.Equal(t, admin.ID, adminSent[0].UserID)
			require.Equal(t, "bob", adminSent[0].Labels["username"])
			require.Equal(t, tc.wantThreshold, adminSent[0].Labels["threshold"])
			require.Equal(t, "$100.00", adminSent[0].Labels["limit"])
			require.Equal(t, "Engineering", adminSent[0].Labels["effective_group_name"])
			require.Equal(t, tc.wantLimitSource, adminSent[0].Labels["limit_source"])
		})
	}
}

// newTestInterception returns an interception with a fixed initiator, provider,
// and model for cost-attribution test setup. The provider name intentionally
// differs from the upstream wire format.
func newTestInterception(id uuid.UUID) database.AIBridgeInterception {
	return database.AIBridgeInterception{
		ID:           id,
		InitiatorID:  uuid.New(),
		Provider:     "anthropic",
		ProviderName: "anthropic-eu",
		Model:        "claude-sonnet-4-6",
	}
}

// expectTokenUsageCostLookups mocks the store lookups made by resolveTokenUsageCost
// (budget resolution, provider resolution, and the price lookup). A nil override, group,
// everyoneGroupID, or price makes that lookup return sql.ErrNoRows. Budget resolution
// mirrors production code: a non-nil override wins and skips the group lookup, and the
// Everyone fallback is consulted only when both override and group are nil. The provider
// name resolves to a provider whose configured provider type equals the interception's
// upstream wire format.
func expectTokenUsageCostLookups(
	db *dbmock.MockStore,
	intc database.AIBridgeInterception,
	override *database.UserAIBudgetOverride,
	group *database.GetHighestGroupAIBudgetByUserRow,
	everyoneGroupID *uuid.UUID,
	price *database.AIModelPrice,
) {
	db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intc.ID).Return(intc, nil)

	if override != nil {
		db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), intc.InitiatorID).Return(*override, nil)
	} else {
		db.EXPECT().GetUserAIBudgetOverride(gomock.Any(), intc.InitiatorID).
			Return(database.UserAIBudgetOverride{}, sql.ErrNoRows)
		if group != nil {
			db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), intc.InitiatorID).Return(*group, nil)
		} else {
			db.EXPECT().GetHighestGroupAIBudgetByUser(gomock.Any(), intc.InitiatorID).
				Return(database.GetHighestGroupAIBudgetByUserRow{}, sql.ErrNoRows)
			if everyoneGroupID != nil {
				db.EXPECT().GetUserEveryoneFallbackGroup(gomock.Any(), intc.InitiatorID).
					Return(*everyoneGroupID, nil)
			} else {
				db.EXPECT().GetUserEveryoneFallbackGroup(gomock.Any(), intc.InitiatorID).
					Return(uuid.Nil, sql.ErrNoRows)
			}
		}
	}

	db.EXPECT().GetAIProviderByName(gomock.Any(), intc.ProviderName).Return(database.AIProvider{
		Name: intc.ProviderName,
		Type: database.AIProviderType(intc.Provider),
	}, nil)

	if price != nil {
		db.EXPECT().GetAIModelPriceByProviderModel(gomock.Any(), database.GetAIModelPriceByProviderModelParams{
			Provider: intc.Provider,
			Model:    intc.Model,
		}).Return(*price, nil)
	} else {
		db.EXPECT().GetAIModelPriceByProviderModel(gomock.Any(), gomock.Any()).
			Return(database.AIModelPrice{}, sql.ErrNoRows)
	}
}

func TestRecordPromptUsage(t *testing.T) {
	t.Parallel()

	var (
		metadataProto = map[string]*anypb.Any{
			"key": mustMarshalAny(t, &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "value"}}),
		}
		metadataJSON = `{"key":"value"}`
	)

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordPromptUsageRequest) (*proto.RecordPromptUsageResponse, error) {
			return srv.RecordPromptUsage(ctx, req)
		},
		[]testRecordMethodCase[*proto.RecordPromptUsageRequest]{
			{
				name: "valid prompt usage",
				request: &proto.RecordPromptUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					Prompt:         "yo",
					Metadata:       metadataProto,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordPromptUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					db.EXPECT().InsertAIBridgeUserPrompt(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeUserPromptParams) bool {
						if !assert.NotEqual(t, uuid.Nil, p.ID, "ID") ||
							!assert.Equal(t, interceptionID, p.InterceptionID, "interception ID") ||
							!assert.Equal(t, req.GetMsgId(), p.ProviderResponseID, "provider response ID") ||
							!assert.Equal(t, req.GetPrompt(), p.Prompt, "prompt") ||
							!assert.JSONEq(t, metadataJSON, string(p.Metadata), "metadata") ||
							!assert.WithinDuration(t, req.GetCreatedAt().AsTime(), p.CreatedAt, time.Second, "created at") {
							return false
						}
						return true
					})).Return(database.AIBridgeUserPrompt{
						ID:                 uuid.New(),
						InterceptionID:     interceptionID,
						ProviderResponseID: req.GetMsgId(),
						Prompt:             req.GetPrompt(),
						Metadata: pqtype.NullRawMessage{
							RawMessage: json.RawMessage(metadataJSON),
							Valid:      true,
						},
						CreatedAt: req.GetCreatedAt().AsTime(),
					}, nil)
				},
			},
			{
				name: "invalid interception ID",
				request: &proto.RecordPromptUsageRequest{
					InterceptionId: "not-a-uuid",
					MsgId:          "msg_123",
					Prompt:         "yo",
					CreatedAt:      timestamppb.Now(),
				},
				expectedErr: "failed to parse interception_id",
			},
			{
				name: "database error",
				request: &proto.RecordPromptUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					Prompt:         "yo",
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordPromptUsageRequest) {
					db.EXPECT().InsertAIBridgeUserPrompt(gomock.Any(), gomock.Any()).Return(database.AIBridgeUserPrompt{}, sql.ErrConnDone)
				},
				expectedErr: "insert user prompt",
			},
		},
	)
}

func TestRecordToolUsage(t *testing.T) {
	t.Parallel()

	var (
		metadataProto = map[string]*anypb.Any{
			"key": mustMarshalAny(t, &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 123.45}}),
		}
		metadataJSON = `{"key":123.45}`
	)

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
			return srv.RecordToolUsage(ctx, req)
		},
		[]testRecordMethodCase[*proto.RecordToolUsageRequest]{
			{
				name: "valid tool usage with all fields",
				request: &proto.RecordToolUsageRequest{
					InterceptionId:  uuid.NewString(),
					MsgId:           "msg_123",
					ToolCallId:      "call_xyz",
					ItemId:          "fc_item_xyz",
					ServerUrl:       ptr.Ref("https://api.example.com"),
					Tool:            "read_file",
					Input:           `{"path": "/etc/hosts"}`,
					Injected:        false,
					InvocationError: ptr.Ref("permission denied"),
					Metadata:        metadataProto,
					CreatedAt:       timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordToolUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					dbServerURL := sql.NullString{}
					if req.ServerUrl != nil {
						dbServerURL.String = *req.ServerUrl
						dbServerURL.Valid = true
					}

					dbInvocationError := sql.NullString{}
					if req.InvocationError != nil {
						dbInvocationError.String = *req.InvocationError
						dbInvocationError.Valid = true
					}

					db.EXPECT().InsertAIBridgeToolUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeToolUsageParams) bool {
						if !assert.NotEqual(t, uuid.Nil, p.ID, "ID") ||
							!assert.Equal(t, interceptionID, p.InterceptionID, "interception ID") ||
							!assert.Equal(t, req.GetMsgId(), p.ProviderResponseID, "provider response ID") ||
							!assert.Equal(t, sql.NullString{String: "call_xyz", Valid: true}, p.ProviderToolCallID, "provider tool call ID") ||
							!assert.Equal(t, sql.NullString{String: "fc_item_xyz", Valid: true}, p.ProviderItemID, "provider item ID") ||
							!assert.Equal(t, req.GetTool(), p.Tool, "tool") ||
							!assert.Equal(t, dbServerURL, p.ServerUrl, "server URL") ||
							!assert.Equal(t, req.GetInput(), p.Input, "input") ||
							!assert.Equal(t, req.GetInjected(), p.Injected, "injected") ||
							!assert.Equal(t, dbInvocationError, p.InvocationError, "invocation error") ||
							!assert.JSONEq(t, metadataJSON, string(p.Metadata), "metadata") ||
							!assert.WithinDuration(t, req.GetCreatedAt().AsTime(), p.CreatedAt, time.Second, "created at") {
							return false
						}
						return true
					})).Return(database.AIBridgeToolUsage{
						ID:                 uuid.New(),
						InterceptionID:     interceptionID,
						ProviderResponseID: req.GetMsgId(),
						Tool:               req.GetTool(),
						ServerUrl:          dbServerURL,
						Input:              req.GetInput(),
						Injected:           req.GetInjected(),
						InvocationError:    dbInvocationError,
						Metadata: pqtype.NullRawMessage{
							RawMessage: json.RawMessage(metadataJSON),
							Valid:      true,
						},
						CreatedAt: req.GetCreatedAt().AsTime(),
					}, nil)
				},
			},
			{
				name: "invalid interception ID",
				request: &proto.RecordToolUsageRequest{
					InterceptionId: "not-a-uuid",
					MsgId:          "msg_123",
					Tool:           "read_file",
					Input:          `{"path": "/etc/hosts"}`,
					CreatedAt:      timestamppb.Now(),
				},
				expectedErr: "failed to parse interception_id",
			},
			{
				name: "database error",
				request: &proto.RecordToolUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					Tool:           "read_file",
					Input:          `{"path": "/etc/hosts"}`,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordToolUsageRequest) {
					db.EXPECT().InsertAIBridgeToolUsage(gomock.Any(), gomock.Any()).Return(database.AIBridgeToolUsage{}, sql.ErrConnDone)
				},
				expectedErr: "insert tool usage",
			},
		},
	)
}

func TestRecordModelThought(t *testing.T) {
	t.Parallel()

	var (
		metadataProto = map[string]*anypb.Any{
			"key": mustMarshalAny(t, &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "value"}}),
		}
		metadataJSON = `{"key":"value"}`
	)

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordModelThoughtRequest) (*proto.RecordModelThoughtResponse, error) {
			return srv.RecordModelThought(ctx, req)
		},
		[]testRecordMethodCase[*proto.RecordModelThoughtRequest]{
			{
				name: "valid model thought",
				request: &proto.RecordModelThoughtRequest{
					InterceptionId: uuid.NewString(),
					Content:        "I should list the files.",
					Metadata:       metadataProto,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordModelThoughtRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					db.EXPECT().InsertAIBridgeModelThought(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeModelThoughtParams) bool {
						if !assert.Equal(t, interceptionID, p.InterceptionID, "interception ID") ||
							!assert.Equal(t, "I should list the files.", p.Content, "content") ||
							!assert.JSONEq(t, metadataJSON, string(p.Metadata), "metadata") {
							return false
						}
						return true
					})).Return(database.AIBridgeModelThought{
						InterceptionID: interceptionID,
						Content:        "I should list the files.",
						Metadata: pqtype.NullRawMessage{
							RawMessage: json.RawMessage(metadataJSON),
							Valid:      true,
						},
					}, nil)
				},
			},
			{
				name: "invalid interception ID",
				request: &proto.RecordModelThoughtRequest{
					InterceptionId: "not-a-uuid",
					Content:        "thinking...",
					CreatedAt:      timestamppb.Now(),
				},
				expectedErr: "failed to parse interception_id",
			},
			{
				name: "database error",
				request: &proto.RecordModelThoughtRequest{
					InterceptionId: uuid.NewString(),
					Content:        "thinking...",
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordModelThoughtRequest) {
					db.EXPECT().InsertAIBridgeModelThought(gomock.Any(), gomock.Any()).Return(database.AIBridgeModelThought{}, sql.ErrConnDone)
				},
				expectedErr: "insert model thought",
			},
		},
	)
}

type testRecordMethodCase[Req any] struct {
	name    string
	request Req
	// setupMocks is called with the mock store and the above request.
	setupMocks  func(t *testing.T, db *dbmock.MockStore, req Req)
	expectedErr string
	// assertMetrics, when set, is called after the method returns to assert
	// the metrics recorded on the server's registry.
	assertMetrics func(t *testing.T, reg *prometheus.Registry)
}

// testRecordMethod is a helper that abstracts the common testing pattern for all Record* methods.
func testRecordMethod[Req any, Resp any](
	t *testing.T,
	callMethod func(srv *aibridgedserver.Server, ctx context.Context, req Req) (Resp, error),
	cases []testRecordMethodCase[Req],
) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := testutil.Logger(t)

			if tc.setupMocks != nil {
				tc.setupMocks(t, db, tc.request)
			}

			ctx := testutil.Context(t, testutil.WaitLong)
			reg := prometheus.NewRegistry()
			metrics := aibridgedserver.NewMetrics(reg)
			srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
				Store:         db,
				AISeatTracker: agplaiseats.Noop{},
				AccessURL:     "/",
				GatewayCfg:    codersdk.AIBridgeConfig{},
				Experiments:   requiredExperiments,
				Logger:        logger,
				Clock:         quartz.NewReal(),
				Metrics:       metrics,
			})
			require.NoError(t, err)

			resp, err := callMethod(srv, ctx, tc.request)
			if tc.expectedErr != "" {
				require.Error(t, err, "Expected error for test case: %s", tc.name)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err, "Unexpected error for test case: %s", tc.name)
				require.NotNil(t, resp)
			}
			if tc.assertMetrics != nil {
				tc.assertMetrics(t, reg)
			}
		})
	}
}

// Helper functions.
func mustMarshalAny(t *testing.T, msg protobufproto.Message) *anypb.Any {
	t.Helper()
	v, err := anypb.New(msg)
	require.NoError(t, err)
	return v
}

// logLine represents a parsed JSON log entry.
type logLine struct {
	Msg    string         `json:"msg"`
	Level  string         `json:"level"`
	Fields map[string]any `json:"fields"`
}

// parseLogLines parses JSON log lines from a buffer.
func parseLogLines(buf *bytes.Buffer) []logLine {
	var lines []logLine
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		var line logLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err == nil {
			lines = append(lines, line)
		}
	}
	return lines
}

// getLogLinesWithMessage returns all log lines with the given message.
func getLogLinesWithMessage(lines []logLine, msg string) []logLine {
	var result []logLine
	for _, line := range lines {
		if line.Msg == msg {
			result = append(result, line)
		}
	}
	return result
}

func TestStructuredLogging(t *testing.T) {
	t.Parallel()

	metadataProto := map[string]*anypb.Any{
		"key": mustMarshalAny(t, &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "value"}}),
	}

	type testCase struct {
		name              string
		structuredLogging bool
		expectedErr       error
		setupMocks        func(db *dbmock.MockStore, interceptionID uuid.UUID)
		recordFn          func(srv *aibridgedserver.Server, ctx context.Context, interceptionID uuid.UUID) error
		expectedFields    map[string]any
	}

	interceptionID := uuid.UUID{1}
	initiatorID := uuid.UUID{2}
	threadParentID := uuid.UUID{3}
	threadRootID := uuid.UUID{4}

	toolCallID := "my-tool-call"
	sessionID := "some-session-id"

	cases := []testCase{
		{
			name:              "RecordInterception_logs_when_enabled",
			structuredLogging: true,
			setupMocks: func(db *dbmock.MockStore, intcID uuid.UUID) {
				db.EXPECT().GetAIBridgeInterceptionLineageByToolCallID(gomock.Any(), toolCallID).Return(database.GetAIBridgeInterceptionLineageByToolCallIDRow{
					ThreadParentID: threadParentID,
					ThreadRootID:   threadRootID,
				}, nil)

				db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Any()).Return(database.AIBridgeInterception{
					ID:             intcID,
					InitiatorID:    initiatorID,
					ThreadParentID: uuid.NullUUID{UUID: threadParentID, Valid: true},
					ThreadRootID:   uuid.NullUUID{UUID: threadRootID, Valid: true},
				}, nil)
			},
			recordFn: func(srv *aibridgedserver.Server, ctx context.Context, intcID uuid.UUID) error {
				_, err := srv.RecordInterception(ctx, &proto.RecordInterceptionRequest{
					Id:                    intcID.String(),
					ApiKeyId:              "api-key-123",
					InitiatorId:           initiatorID.String(),
					Provider:              "anthropic",
					Model:                 "claude-4-opus",
					Metadata:              metadataProto,
					StartedAt:             timestamppb.Now(),
					CorrelatingToolCallId: ptr.Ref(toolCallID),
					ClientSessionId:       ptr.Ref(sessionID),
				})

				return err
			},
			expectedFields: map[string]any{
				"record_type":              "interception_start",
				"interception_id":          interceptionID.String(),
				"initiator_id":             initiatorID.String(),
				"provider":                 "anthropic",
				"model":                    "claude-4-opus",
				"correlating_tool_call_id": toolCallID,
				"thread_parent_id":         threadParentID.String(),
				"thread_root_id":           threadRootID.String(),
				"client_session_id":        sessionID,
			},
		},
		{
			name:              "RecordInterception_does_not_log_when_disabled",
			structuredLogging: false,
			setupMocks: func(db *dbmock.MockStore, intcID uuid.UUID) {
				db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Any()).Return(database.AIBridgeInterception{
					ID:          intcID,
					InitiatorID: initiatorID,
				}, nil)
			},
			recordFn: func(srv *aibridgedserver.Server, ctx context.Context, intcID uuid.UUID) error {
				_, err := srv.RecordInterception(ctx, &proto.RecordInterceptionRequest{
					Id:          intcID.String(),
					ApiKeyId:    "api-key-123",
					InitiatorId: initiatorID.String(),
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					StartedAt:   timestamppb.Now(),
				})
				return err
			},
			expectedFields: nil, // No log expected.
		},
		{
			name:              "RecordInterception_log_on_db_error",
			structuredLogging: true,
			expectedErr:       sql.ErrConnDone,
			setupMocks: func(db *dbmock.MockStore, intcID uuid.UUID) {
				db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Any()).Return(database.AIBridgeInterception{}, sql.ErrConnDone)
			},
			recordFn: func(srv *aibridgedserver.Server, ctx context.Context, intcID uuid.UUID) error {
				_, err := srv.RecordInterception(ctx, &proto.RecordInterceptionRequest{
					Id:          intcID.String(),
					ApiKeyId:    "api-key-123",
					InitiatorId: initiatorID.String(),
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					StartedAt:   timestamppb.Now(),
				})
				return err
			},
			// Even though the database call errored, we must still write the logs.
			expectedFields: map[string]any{
				"record_type":     "interception_start",
				"interception_id": interceptionID.String(),
				"initiator_id":    initiatorID.String(),
				"provider":        "anthropic",
				"model":           "claude-4-opus",
			},
		},
		{
			name:              "RecordInterceptionEnded_logs_when_enabled",
			structuredLogging: true,
			setupMocks: func(db *dbmock.MockStore, intcID uuid.UUID) {
				db.EXPECT().UpdateAIBridgeInterceptionEnded(gomock.Any(), gomock.Any()).Return(database.AIBridgeInterception{
					ID: intcID,
				}, nil)
			},
			recordFn: func(srv *aibridgedserver.Server, ctx context.Context, intcID uuid.UUID) error {
				_, err := srv.RecordInterceptionEnded(ctx, &proto.RecordInterceptionEndedRequest{
					Id:      intcID.String(),
					EndedAt: timestamppb.Now(),
				})
				return err
			},
			expectedFields: map[string]any{
				"record_type":     "interception_end",
				"interception_id": interceptionID.String(),
			},
		},
		{
			name:              "RecordTokenUsage_logs_when_enabled",
			structuredLogging: true,
			setupMocks: func(db *dbmock.MockStore, intcID uuid.UUID) {
				everyoneID := uuid.New()
				expectTokenUsageCostLookups(db, newTestInterception(intcID), nil, nil, &everyoneID, nil)
				db.EXPECT().InTx(gomock.Any(), nil).DoAndReturn(
					func(fn func(database.Store) error, _ *database.TxOptions) error { return fn(db) },
				)
				db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).Return(database.AIBridgeTokenUsage{
					ID:             uuid.New(),
					InterceptionID: intcID,
				}, nil)
			},
			recordFn: func(srv *aibridgedserver.Server, ctx context.Context, intcID uuid.UUID) error {
				_, err := srv.RecordTokenUsage(ctx, &proto.RecordTokenUsageRequest{
					InterceptionId:        intcID.String(),
					MsgId:                 "msg_123",
					InputTokens:           100,
					OutputTokens:          200,
					CacheReadInputTokens:  50,
					CacheWriteInputTokens: 10,
					Metadata:              metadataProto,
					CreatedAt:             timestamppb.Now(),
				})
				return err
			},
			expectedFields: map[string]any{
				"record_type":              "token_usage",
				"interception_id":          interceptionID.String(),
				"input_tokens":             float64(100), // JSON numbers are float64.
				"output_tokens":            float64(200),
				"cache_read_input_tokens":  float64(50),
				"cache_write_input_tokens": float64(10),
			},
		},
		{
			name:              "RecordPromptUsage_logs_when_enabled",
			structuredLogging: true,
			setupMocks: func(db *dbmock.MockStore, intcID uuid.UUID) {
				db.EXPECT().InsertAIBridgeUserPrompt(gomock.Any(), gomock.Any()).Return(database.AIBridgeUserPrompt{
					ID:             uuid.New(),
					InterceptionID: intcID,
				}, nil)
			},
			recordFn: func(srv *aibridgedserver.Server, ctx context.Context, intcID uuid.UUID) error {
				_, err := srv.RecordPromptUsage(ctx, &proto.RecordPromptUsageRequest{
					InterceptionId: intcID.String(),
					MsgId:          "msg_123",
					Prompt:         "Hello, Claude!",
					Metadata:       metadataProto,
					CreatedAt:      timestamppb.Now(),
				})
				return err
			},
			expectedFields: map[string]any{
				"record_type":     "prompt_usage",
				"interception_id": interceptionID.String(),
				"prompt":          "Hello, Claude!",
			},
		},
		{
			name:              "RecordToolUsage_logs_when_enabled",
			structuredLogging: true,
			setupMocks: func(db *dbmock.MockStore, intcID uuid.UUID) {
				db.EXPECT().InsertAIBridgeToolUsage(gomock.Any(), gomock.Any()).Return(database.AIBridgeToolUsage{
					ID:             uuid.New(),
					InterceptionID: intcID,
				}, nil)
			},
			recordFn: func(srv *aibridgedserver.Server, ctx context.Context, intcID uuid.UUID) error {
				_, err := srv.RecordToolUsage(ctx, &proto.RecordToolUsageRequest{
					InterceptionId:  intcID.String(),
					MsgId:           "msg_123",
					ServerUrl:       ptr.Ref("https://api.example.com"),
					Tool:            "read_file",
					Input:           `{"path": "/etc/hosts"}`,
					Injected:        true,
					InvocationError: ptr.Ref("permission denied"),
					Metadata:        metadataProto,
					CreatedAt:       timestamppb.Now(),
				})
				return err
			},
			expectedFields: map[string]any{
				"record_type":      "tool_usage",
				"interception_id":  interceptionID.String(),
				"tool":             "read_file",
				"input":            `{"path": "/etc/hosts"}`,
				"injected":         true,
				"invocation_error": "permission denied",
			},
		},
		{
			name:              "RecordModelThought_logs_when_enabled",
			structuredLogging: true,
			setupMocks: func(db *dbmock.MockStore, intcID uuid.UUID) {
				db.EXPECT().InsertAIBridgeModelThought(gomock.Any(), gomock.Any()).Return(database.AIBridgeModelThought{
					InterceptionID: intcID,
				}, nil)
			},
			recordFn: func(srv *aibridgedserver.Server, ctx context.Context, intcID uuid.UUID) error {
				_, err := srv.RecordModelThought(ctx, &proto.RecordModelThoughtRequest{
					InterceptionId: intcID.String(),
					Content:        "I need to list the files.",
					Metadata:       metadataProto,
					CreatedAt:      timestamppb.Now(),
				})
				return err
			},
			expectedFields: map[string]any{
				"record_type":     "model_thought",
				"interception_id": interceptionID.String(),
				"content":         "I need to list the files.",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			buf := &bytes.Buffer{}
			logger := slog.Make(slogjson.Sink(buf)).Leveled(slog.LevelDebug)

			tc.setupMocks(db, interceptionID)

			ctx := testutil.Context(t, testutil.WaitLong)
			srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
				Store:         db,
				AISeatTracker: agplaiseats.Noop{},
				AccessURL:     "/",
				GatewayCfg: codersdk.AIBridgeConfig{
					StructuredLogging: serpent.Bool(tc.structuredLogging),
				},
				Experiments: requiredExperiments,
				Logger:      logger,
				Clock:       quartz.NewReal(),
			})
			require.NoError(t, err)

			err = tc.recordFn(srv, ctx, interceptionID)
			if tc.expectedErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			lines := parseLogLines(buf)
			if tc.expectedFields == nil {
				// No log expected (disabled or error case).
				require.Empty(t, lines)
			} else {
				matchedLines := getLogLinesWithMessage(lines, aibridgedserver.InterceptionLogMarker)
				require.GreaterOrEqual(t, len(matchedLines), 1, "expected at least 1 log line(s) with message %q", aibridgedserver.InterceptionLogMarker)

				fields := matchedLines[0].Fields
				for key, expected := range tc.expectedFields {
					require.Equal(t, expected, fields[key], "field %q mismatch", key)
				}
			}
		})
	}
}

// TestInferredThreadsByToolCalls verifies that a chain of interceptions linked via
// tool call IDs correctly propagates thread_parent_id and thread_root_id.
//
// The chain is: A → B → C
//   - A is the root (no parent, no root)
//   - B correlates via a tool call recorded by A (parent=A, root=A)
//   - C correlates via a tool call recorded by B (parent=B, root=A)
func TestInferredThreadsByToolCalls(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)

	user := dbgen.User(t, db, database.User{})

	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         db,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Experiments:   requiredExperiments,
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	aID := uuid.New()
	bID := uuid.New()
	cID := uuid.New()

	// Record interception A (root of the chain, no correlation).
	_, err = srv.RecordInterception(ctx, &proto.RecordInterceptionRequest{
		Id:          aID.String(),
		ApiKeyId:    uuid.NewString(),
		InitiatorId: user.ID.String(),
		Provider:    "anthropic",
		Model:       "claude-4-opus",
		StartedAt:   timestamppb.Now(),
	})
	require.NoError(t, err)

	// No thread association yet.
	intcA, err := db.GetAIBridgeInterceptionByID(ctx, aID)
	require.NoError(t, err)
	require.Equal(t, uuid.NullUUID{}, intcA.ThreadParentID)
	require.Equal(t, uuid.NullUUID{}, intcA.ThreadRootID)

	// Record tool usage on A with a known tool call ID.
	_, err = srv.RecordToolUsage(ctx, &proto.RecordToolUsageRequest{
		InterceptionId: aID.String(),
		MsgId:          "resp_a",
		ToolCallId:     "call_a",
		Tool:           "bash",
		Input:          "{}",
		CreatedAt:      timestamppb.Now(),
	})
	require.NoError(t, err)

	// Record interception B correlating to A's tool call.
	_, err = srv.RecordInterception(ctx, &proto.RecordInterceptionRequest{
		Id:                    bID.String(),
		ApiKeyId:              uuid.NewString(),
		InitiatorId:           user.ID.String(),
		Provider:              "anthropic",
		Model:                 "claude-4-opus",
		StartedAt:             timestamppb.Now(),
		CorrelatingToolCallId: ptr.Ref("call_a"),
	})
	require.NoError(t, err)

	intcB, err := db.GetAIBridgeInterceptionByID(ctx, bID)
	require.NoError(t, err)
	require.Equal(t, uuid.NullUUID{UUID: aID, Valid: true}, intcB.ThreadParentID)
	require.Equal(t, uuid.NullUUID{UUID: aID, Valid: true}, intcB.ThreadRootID)

	// Record tool usage on B.
	_, err = srv.RecordToolUsage(ctx, &proto.RecordToolUsageRequest{
		InterceptionId: bID.String(),
		MsgId:          "resp_b",
		ToolCallId:     "call_b",
		Tool:           "bash",
		Input:          "{}",
		CreatedAt:      timestamppb.Now(),
	})
	require.NoError(t, err)

	// Record interception C correlating to B's tool call.
	_, err = srv.RecordInterception(ctx, &proto.RecordInterceptionRequest{
		Id:                    cID.String(),
		ApiKeyId:              uuid.NewString(),
		InitiatorId:           user.ID.String(),
		Provider:              "anthropic",
		Model:                 "claude-4-opus",
		StartedAt:             timestamppb.Now(),
		CorrelatingToolCallId: ptr.Ref("call_b"),
	})
	require.NoError(t, err)

	intcC, err := db.GetAIBridgeInterceptionByID(ctx, cID)
	require.NoError(t, err)
	require.Equal(t, uuid.NullUUID{UUID: bID, Valid: true}, intcC.ThreadParentID)
	require.Equal(t, uuid.NullUUID{UUID: aID, Valid: true}, intcC.ThreadRootID)
}

// TestRecordToolUsageProviderItemID exercises the RecordToolUsage RPC against a
// real database and confirms that provider_item_id is persisted in its own
// column for both shapes of Responses-API tool call. Agentic tools carry both
// an item id and a tool_call_id; hosted tools (e.g. web_search_call) carry only
// an item id. The hosted case is the important one: it proves the item id is
// stored even when tool_call_id is absent, so persistence is not gated on the
// tool_call_id being present, and the two ids are written to their own columns.
func TestRecordToolUsageProviderItemID(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)

	user := dbgen.User(t, db, database.User{})

	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         db,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Experiments:   requiredExperiments,
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	intcID := uuid.New()
	_, err = srv.RecordInterception(ctx, &proto.RecordInterceptionRequest{
		Id:          intcID.String(),
		ApiKeyId:    uuid.NewString(),
		InitiatorId: user.ID.String(),
		Provider:    "openai",
		Model:       "gpt-5",
		StartedAt:   timestamppb.Now(),
	})
	require.NoError(t, err)

	// Agentic tool: both item_id and tool_call_id are present.
	_, err = srv.RecordToolUsage(ctx, &proto.RecordToolUsageRequest{
		InterceptionId: intcID.String(),
		MsgId:          "resp_1",
		ToolCallId:     "call_agentic",
		ItemId:         "fc_item_1",
		Tool:           "function_call",
		Input:          "{}",
		CreatedAt:      timestamppb.Now(),
	})
	require.NoError(t, err)

	// Hosted tool: only item_id is present, tool_call_id is empty.
	_, err = srv.RecordToolUsage(ctx, &proto.RecordToolUsageRequest{
		InterceptionId: intcID.String(),
		MsgId:          "resp_1",
		ItemId:         "ws_item_1",
		Tool:           "web_search_call",
		Input:          "{}",
		CreatedAt:      timestamppb.Now(),
	})
	require.NoError(t, err)

	usages, err := db.GetAIBridgeToolUsagesByInterceptionID(ctx, intcID)
	require.NoError(t, err)
	require.Len(t, usages, 2)

	byItemID := make(map[string]database.AIBridgeToolUsage, len(usages))
	for _, u := range usages {
		require.True(t, u.ProviderItemID.Valid, "item ID should be persisted for %q", u.Tool)
		byItemID[u.ProviderItemID.String] = u
	}

	// Agentic tool: item id and tool_call_id land in their own columns.
	agentic, ok := byItemID["fc_item_1"]
	require.True(t, ok, "agentic tool usage persisted by item ID")
	require.Equal(t, sql.NullString{String: "call_agentic", Valid: true}, agentic.ProviderToolCallID)

	// Hosted tool: item id is persisted even though the tool_call_id is empty.
	hosted, ok := byItemID["ws_item_1"]
	require.True(t, ok, "hosted tool usage persisted by item ID")
	require.Equal(t, sql.NullString{}, hosted.ProviderToolCallID, "hosted tool has no tool_call_id")
}

// TestGetAIProviders exercises the row-to-proto mapping over a real database:
// enabled providers carry their keys (and typed Bedrock settings), disabled
// providers are included but withhold keys and settings, Copilot (a keyless
// BYOK provider) round-trips with no keys, and an enabled provider whose
// settings blob cannot be decoded is skipped rather than failing the fetch.
func TestGetAIProviders(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	// The skipped misconfigured provider is logged at Error level by design,
	// so error logs are expected here.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// Enabled OpenAI with two keys.
	openai := dbgen.AIProvider(t, db, database.AIProvider{
		Type:    database.AIProviderTypeOpenai,
		Name:    "openai",
		Enabled: true,
		BaseUrl: "https://api.openai.com/",
	})
	dbgen.AIProviderKey(t, db, database.AIProviderKey{ProviderID: openai.ID, APIKey: "sk-openai-1"})
	dbgen.AIProviderKey(t, db, database.AIProviderKey{ProviderID: openai.ID, APIKey: "sk-openai-2"})

	// Enabled Bedrock with typed settings.
	bedrockSettings, err := json.Marshal(codersdk.AIProviderSettings{
		Bedrock: &codersdk.AIProviderBedrockSettings{
			Region:          "us-east-1",
			Model:           "anthropic.claude-3",
			SmallFastModel:  "anthropic.claude-haiku",
			AccessKey:       ptr.Ref("AKID"),
			AccessKeySecret: ptr.Ref("secret"),
			RoleARN:         "arn:aws:iam::123456789012:role/bedrock",
		},
	})
	require.NoError(t, err)
	dbgen.AIProvider(t, db, database.AIProvider{
		Type:     database.AIProviderTypeBedrock,
		Name:     "bedrock",
		Enabled:  true,
		BaseUrl:  "https://bedrock-runtime.us-east-1.amazonaws.com/",
		Settings: sql.NullString{String: string(bedrockSettings), Valid: true},
	})

	// Enabled Copilot, which is keyless (BYOK per request).
	dbgen.AIProvider(t, db, database.AIProvider{
		Type:    database.AIProviderTypeCopilot,
		Name:    "copilot",
		Enabled: true,
		BaseUrl: "https://api.githubcopilot.com/",
	})

	// Disabled Anthropic with a key; the key must be withheld.
	disabled := dbgen.AIProvider(t, db, database.AIProvider{
		Type:    database.AIProviderTypeAnthropic,
		Name:    "anthropic-off",
		BaseUrl: "https://api.anthropic.com/",
	}, func(p *database.InsertAIProviderParams) {
		p.Enabled = false
	})
	dbgen.AIProviderKey(t, db, database.AIProviderKey{ProviderID: disabled.ID, APIKey: "sk-secret"})

	// Enabled provider with an undecodable settings blob; it must be skipped
	// so one corrupt row does not break provider config for every gateway.
	dbgen.AIProvider(t, db, database.AIProvider{
		Type:     database.AIProviderTypeBedrock,
		Name:     "broken-settings",
		Enabled:  true,
		BaseUrl:  "https://bedrock-runtime.us-east-1.amazonaws.com/",
		Settings: sql.NullString{String: "{not valid json", Valid: true},
	})

	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         db,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	resp, err := srv.GetAIProviders(ctx, &proto.GetAIProvidersRequest{})
	require.NoError(t, err)

	byName := make(map[string]*proto.AIProvider, len(resp.GetProviders()))
	for _, p := range resp.GetProviders() {
		byName[p.GetName()] = p
	}
	require.Len(t, byName, 4)
	assert.NotContains(t, byName, "broken-settings", "provider with undecodable settings must be skipped")

	gotOpenAI := byName["openai"]
	require.NotNil(t, gotOpenAI)
	assert.True(t, gotOpenAI.GetEnabled())
	assert.Equal(t, string(database.AIProviderTypeOpenai), gotOpenAI.GetType())
	assert.Equal(t, "https://api.openai.com/", gotOpenAI.GetBaseUrl())
	assert.ElementsMatch(t, []string{"sk-openai-1", "sk-openai-2"}, gotOpenAI.GetKeys())
	assert.Nil(t, gotOpenAI.GetBedrock())

	gotBedrock := byName["bedrock"]
	require.NotNil(t, gotBedrock)
	assert.True(t, gotBedrock.GetEnabled())
	require.NotNil(t, gotBedrock.GetBedrock())
	assert.Equal(t, "us-east-1", gotBedrock.GetBedrock().GetRegion())
	assert.Equal(t, "anthropic.claude-3", gotBedrock.GetBedrock().GetModel())
	assert.Equal(t, "anthropic.claude-haiku", gotBedrock.GetBedrock().GetSmallFastModel())
	assert.Equal(t, "AKID", gotBedrock.GetBedrock().GetAccessKey())
	assert.Equal(t, "secret", gotBedrock.GetBedrock().GetAccessKeySecret())
	assert.Equal(t, "arn:aws:iam::123456789012:role/bedrock", gotBedrock.GetBedrock().GetRoleArn())

	gotCopilot := byName["copilot"]
	require.NotNil(t, gotCopilot)
	assert.True(t, gotCopilot.GetEnabled())
	assert.Empty(t, gotCopilot.GetKeys())

	gotDisabled := byName["anthropic-off"]
	require.NotNil(t, gotDisabled)
	assert.False(t, gotDisabled.GetEnabled())
	assert.Empty(t, gotDisabled.GetKeys(), "keys must be withheld for disabled providers")
	assert.Nil(t, gotDisabled.GetBedrock())
}

// TestGetAIProvidersBlocksOnSeedLock asserts that GetAIProviders serializes on
// LockIDAIProvidersEnvSeed: while an in-flight seed transaction holds the lock,
// the fetch blocks, and once the seed commits the fetch returns the seeded
// set. Postgres advisory locks are required, so this cannot run against the
// mock store.
func TestGetAIProvidersBlocksOnSeedLock(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil)

	dbgen.AIProviderWithOptionalKey(t, db, database.AIProvider{
		Type:    database.AIProviderTypeOpenai,
		Name:    "openai",
		Enabled: true,
		BaseUrl: "https://api.openai.com/",
	}, "sk-openai")

	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         db,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	// Simulate an in-flight env seed holding the advisory lock until released.
	holderReady := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderDone := make(chan struct{})
	go func() {
		defer close(holderDone)
		txErr := db.InTx(func(tx database.Store) error {
			if err := tx.AcquireLock(ctx, database.LockIDAIProvidersEnvSeed); err != nil {
				return err
			}
			close(holderReady)
			<-releaseHolder
			return nil
		}, nil)
		assert.NoError(t, txErr)
	}()

	testutil.TryReceive(ctx, t, holderReady)

	fetchDone := make(chan *proto.GetAIProvidersResponse, 1)
	fetchErr := make(chan error, 1)
	go func() {
		resp, err := srv.GetAIProviders(ctx, &proto.GetAIProvidersRequest{})
		fetchErr <- err
		fetchDone <- resp
	}()

	// Wait until the fetch goroutine is observably blocked waiting on the seed
	// advisory lock, rather than inferring it from a fixed delay. AcquireLock
	// uses the single-bigint advisory lock form, so the waiter appears in
	// pg_locks as an ungranted "advisory" row whose objid is the low 32 bits of
	// the lock ID. Asserting the wait directly stops this from passing vacuously
	// if the goroutine has not yet reached the lock.
	require.Eventually(t, func() bool {
		locks, err := db.PGLocks(ctx)
		if err != nil {
			return false
		}
		for _, l := range locks {
			if l.LockType != nil && *l.LockType == "advisory" && !l.Granted &&
				l.ObjID != nil && *l.ObjID == strconv.Itoa(database.LockIDAIProvidersEnvSeed) {
				return true
			}
		}
		return false
	}, testutil.WaitShort, testutil.IntervalFast, "fetch must block waiting on the seed advisory lock")

	// With the fetch proven to be blocked on the lock, it must not have
	// completed while the lock is still held.
	select {
	case <-fetchDone:
		t.Fatal("GetAIProviders returned before the seed lock was released")
	default:
	}

	// Release the lock; the fetch should now complete and return the seeded set.
	close(releaseHolder)
	testutil.TryReceive(ctx, t, holderDone)

	require.NoError(t, testutil.TryReceive(ctx, t, fetchErr))
	resp := testutil.TryReceive(ctx, t, fetchDone)
	require.Len(t, resp.GetProviders(), 1)
	assert.Equal(t, "openai", resp.GetProviders()[0].GetName())
	assert.Equal(t, []string{"sk-openai"}, resp.GetProviders()[0].GetKeys())
}

// TestWatchAIProviders asserts that the WatchAIProviders handler emits an
// initial signal on subscribe, one signal per AIProvidersChangedChannel publish,
// and returns cleanly when the stream context is canceled.
func TestWatchAIProviders(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil)
	// In-memory pubsub delivers Publish synchronously for deterministic signals.
	ps := pubsub.NewInMemory()

	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         db,
		Pubsub:        ps,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()
	stream := &fakeWatchProvidersStream{ctx: streamCtx, sent: make(chan struct{}, 16)}

	watchErr := make(chan error, 1)
	go func() {
		watchErr <- srv.WatchAIProviders(&proto.WatchAIProvidersRequest{}, stream)
	}()

	// The handler sends an initial signal immediately on subscribe. Draining it
	// before publishing guarantees the next publish is not coalesced into the
	// initial signal.
	testutil.TryReceive(ctx, t, stream.sent)

	require.NoError(t, ps.Publish(coderdpubsub.AIProvidersChangedChannel, nil))
	testutil.TryReceive(ctx, t, stream.sent)

	require.NoError(t, ps.Publish(coderdpubsub.AIProvidersChangedChannel, nil))
	testutil.TryReceive(ctx, t, stream.sent)

	streamCancel()
	require.NoError(t, testutil.TryReceive(ctx, t, watchErr))
}

// TestWatchAIProvidersSignalsOnDeliveryError asserts that a dropped-message
// delivery error is forwarded as a change signal rather than failing the
// stream, so the gateway reconverges after a pubsub drop.
func TestWatchAIProvidersSignalsOnDeliveryError(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil)
	ps := &captureListenerPubsub{listenerC: make(chan pubsub.ListenerWithErr, 1)}

	srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
		Store:         db,
		Pubsub:        ps,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()
	stream := &fakeWatchProvidersStream{ctx: streamCtx, sent: make(chan struct{}, 16)}

	watchErr := make(chan error, 1)
	go func() {
		watchErr <- srv.WatchAIProviders(&proto.WatchAIProvidersRequest{}, stream)
	}()

	// Capture the registered listener and drain the initial subscribe signal so
	// the delivery-error signal that follows is not coalesced into it.
	listener := testutil.TryReceive(ctx, t, ps.listenerC)
	testutil.TryReceive(ctx, t, stream.sent)

	// A delivery error must still produce a signal, exercising the pubsub-error
	// branch of the handler.
	listener(ctx, nil, pubsub.ErrDroppedMessages)
	testutil.TryReceive(ctx, t, stream.sent)

	streamCancel()
	require.NoError(t, testutil.TryReceive(ctx, t, watchErr))
}

// TestWatchAIProvidersStopsOnLifecycleCancel asserts the handler returns when
// the server lifecycle context is canceled even though the stream context
// remains open, so a stream that outlives the server does not leak a goroutine
// on shutdown.
func TestWatchAIProvidersStopsOnLifecycleCancel(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil)
	ps := pubsub.NewInMemory()

	// The lifecycle context is independent of the stream context so it can be
	// canceled while the stream stays open.
	lifecycleCtx, lifecycleCancel := context.WithCancel(ctx)
	defer lifecycleCancel()
	srv, err := aibridgedserver.NewServer(lifecycleCtx, aibridgedserver.Options{
		Store:         db,
		Pubsub:        ps,
		AISeatTracker: agplaiseats.Noop{},
		AccessURL:     "/",
		GatewayCfg:    codersdk.AIBridgeConfig{},
		Logger:        logger,
		Clock:         quartz.NewReal(),
	})
	require.NoError(t, err)

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()
	stream := &fakeWatchProvidersStream{ctx: streamCtx, sent: make(chan struct{}, 16)}

	watchErr := make(chan error, 1)
	go func() {
		watchErr <- srv.WatchAIProviders(&proto.WatchAIProvidersRequest{}, stream)
	}()

	// Drain the initial subscribe signal to confirm the handler is running
	// before the lifecycle is canceled.
	testutil.TryReceive(ctx, t, stream.sent)

	// Canceling only the lifecycle context must stop the handler even though
	// the stream context is still open.
	lifecycleCancel()
	require.NoError(t, testutil.TryReceive(ctx, t, watchErr))
}

var _ pubsub.Pubsub = (*captureListenerPubsub)(nil)

// captureListenerPubsub captures the ListenerWithErr registered via
// SubscribeWithErr so a test can drive delivery (including errors) directly.
type captureListenerPubsub struct {
	listenerC chan pubsub.ListenerWithErr
}

func (*captureListenerPubsub) Subscribe(string, pubsub.Listener) (func(), error) {
	return nil, xerrors.New("Subscribe not implemented")
}

func (p *captureListenerPubsub) SubscribeWithErr(_ string, listener pubsub.ListenerWithErr) (func(), error) {
	p.listenerC <- listener
	return func() {}, nil
}

func (*captureListenerPubsub) Publish(string, []byte) error {
	return xerrors.New("Publish not implemented")
}

func (*captureListenerPubsub) Close() error { return nil }

// fakeWatchProvidersStream is a minimal proto.DRPCProviderConfigurator_WatchAIProvidersStream
// that records Send calls on a channel.
type fakeWatchProvidersStream struct {
	ctx  context.Context
	sent chan struct{}
}

func (s *fakeWatchProvidersStream) Send(*proto.WatchAIProvidersResponse) error {
	select {
	case s.sent <- struct{}{}:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *fakeWatchProvidersStream) Context() context.Context                { return s.ctx }
func (*fakeWatchProvidersStream) MsgSend(drpc.Message, drpc.Encoding) error { return nil }
func (*fakeWatchProvidersStream) MsgRecv(drpc.Message, drpc.Encoding) error { return nil }
func (*fakeWatchProvidersStream) CloseSend() error                          { return nil }
func (*fakeWatchProvidersStream) Close() error                              { return nil }

// countingSeatTracker records the number of RecordUsage calls.
type countingSeatTracker struct {
	calls atomic.Int64
}

func (c *countingSeatTracker) RecordUsage(context.Context, uuid.UUID, agplaiseats.Reason) {
	c.calls.Add(1)
}

// TestRecordInterceptionAISeat verifies that bridge usage claims an AI
// Governance seat only when the seat exclusion experiment is disabled.
func TestRecordInterceptionAISeat(t *testing.T) {
	t.Parallel()

	newRequest := func() *proto.RecordInterceptionRequest {
		return &proto.RecordInterceptionRequest{
			Id:          uuid.NewString(),
			ApiKeyId:    uuid.NewString(),
			InitiatorId: uuid.NewString(),
			Provider:    "anthropic",
			Model:       "claude-4-opus",
			StartedAt:   timestamppb.Now(),
		}
	}

	cases := []struct {
		name          string
		experiments   []codersdk.Experiment
		expectedCalls int64
	}{
		{
			name:          "experiment off records a seat",
			experiments:   requiredExperiments,
			expectedCalls: 1,
		},
		{
			name:          "seat exclusion skips the seat",
			experiments:   append([]codersdk.Experiment{codersdk.ExperimentAIGatewaySeatExclusion}, requiredExperiments...),
			expectedCalls: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Any()).
				Return(database.AIBridgeInterception{}, nil)

			tracker := &countingSeatTracker{}
			ctx := testutil.Context(t, testutil.WaitLong)
			srv, err := aibridgedserver.NewServer(ctx, aibridgedserver.Options{
				Store:         db,
				AISeatTracker: tracker,
				AccessURL:     "/",
				GatewayCfg:    codersdk.AIBridgeConfig{},
				Experiments:   tc.experiments,
				Logger:        testutil.Logger(t),
				Clock:         quartz.NewReal(),
			})
			require.NoError(t, err)

			_, err = srv.RecordInterception(ctx, newRequest())
			require.NoError(t, err)
			require.Equal(t, tc.expectedCalls, tracker.calls.Load())
		})
	}
}
