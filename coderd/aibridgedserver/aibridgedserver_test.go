package aibridgedserver_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	protobufproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/aibridgedserver"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
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
		name        string
		key         string
		mocksFn     func(db *dbmock.MockStore, keyID string, userID uuid.UUID)
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
			mocksFn: func(db *dbmock.MockStore, keyID string, userID uuid.UUID) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).MinTimes(1).Return(database.APIKey{}, sql.ErrNoRows)
			},
		},
		{
			name:        "expired",
			expectedErr: aibridgedserver.ErrExpired,
			mocksFn: func(db *dbmock.MockStore, keyID string, userID uuid.UUID) {
				now := dbtime.Now()
				db.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).MinTimes(1).Return(database.APIKey{ID: keyID, ExpiresAt: now.Add(-time.Hour)}, nil)
			},
		},
		{
			name:        "unknown user",
			expectedErr: aibridgedserver.ErrUnknownUser,
			mocksFn: func(db *dbmock.MockStore, keyID string, userID uuid.UUID) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).MinTimes(1).Return(database.APIKey{ID: keyID, UserID: userID, ExpiresAt: dbtime.Now().Add(time.Hour)}, nil)
				db.EXPECT().GetUserByID(gomock.Any(), userID).MinTimes(1).Return(database.User{}, sql.ErrNoRows)
			},
		},
		{
			name:        "deleted user",
			expectedErr: aibridgedserver.ErrDeletedUser,
			mocksFn: func(db *dbmock.MockStore, keyID string, userID uuid.UUID) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).MinTimes(1).Return(database.APIKey{ID: keyID, UserID: userID, ExpiresAt: dbtime.Now().Add(time.Hour)}, nil)
				db.EXPECT().GetUserByID(gomock.Any(), userID).MinTimes(1).Return(database.User{ID: userID, Deleted: true}, nil)
			},
		},
		{
			name:        "system user",
			expectedErr: aibridgedserver.ErrSystemUser,
			mocksFn: func(db *dbmock.MockStore, keyID string, userID uuid.UUID) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).MinTimes(1).Return(database.APIKey{ID: keyID, UserID: userID, ExpiresAt: dbtime.Now().Add(time.Hour)}, nil)
				db.EXPECT().GetUserByID(gomock.Any(), userID).MinTimes(1).Return(database.User{ID: userID, IsSystem: true}, nil)
			},
		},
		{
			name: "valid",
			mocksFn: func(db *dbmock.MockStore, keyID string, userID uuid.UUID) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).MinTimes(1).Return(database.APIKey{ID: keyID, UserID: userID, ExpiresAt: dbtime.Now().Add(time.Hour)}, nil)
				db.EXPECT().GetUserByID(gomock.Any(), userID).MinTimes(1).Return(database.User{ID: userID}, nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := testutil.Logger(t)

			// Mock the call to insert an API key since dbgen seems to be the best util to use to fake an API key.
			db.EXPECT().InsertAPIKey(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(ctx context.Context, arg database.InsertAPIKeyParams) (database.APIKey, error) {
				return database.APIKey{ID: arg.ID}, nil
			})
			// Key is empty, generate one.

			key, token := dbgen.APIKey(t, db, database.APIKey{})
			if tc.key == "" {
				tc.key = token
			}

			db.EXPECT().InsertUser(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(ctx context.Context, arg database.InsertUserParams) (database.User, error) {
				return database.User{ID: arg.ID}, nil
			})
			db.EXPECT().UpdateUserStatus(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(ctx context.Context, arg database.UpdateUserStatusParams) (database.User, error) {
				return database.User{ID: arg.ID}, nil
			})
			user := dbgen.User(t, db, database.User{})

			// Define any case-specific mocks.
			if tc.mocksFn != nil {
				tc.mocksFn(db, key.ID, user.ID)
			}

			srv, err := aibridgedserver.NewServer(t.Context(), db, logger, "/", nil, requiredExperiments)
			require.NoError(t, err)
			require.NotNil(t, srv)

			_, err = srv.IsAuthorized(t.Context(), &proto.IsAuthorizedRequest{Key: tc.key})
			if tc.expectedErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
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
		name                string
		experiments         codersdk.Experiments
		externalAuthConfigs []*externalauth.Config
		expectCoderMCP      bool
		expectedExternalMCP bool
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := testutil.Logger(t)

			srv, err := aibridgedserver.NewServer(t.Context(), db, logger, "/", tc.externalAuthConfigs, tc.experiments)
			require.NoError(t, err)
			require.NotNil(t, srv)

			resp, err := srv.GetMCPServerConfigs(t.Context(), &proto.GetMCPServerConfigsRequest{})
			require.NoError(t, err)
			require.NotNil(t, resp)

			if tc.expectCoderMCP {
				require.NotNil(t, resp.GetCoderMcpConfig())
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
	srv, err := aibridgedserver.NewServer(t.Context(), db, logger, "/", []*externalauth.Config{
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
	}, requiredExperiments)
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

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordInterceptionRequest) (*proto.RecordInterceptionResponse, error) {
			return srv.RecordInterception(ctx, req)
		},
		[]struct {
			name        string
			request     *proto.RecordInterceptionRequest
			setupMocks  func(*dbmock.MockStore)
			expectedErr string
		}{
			{
				name: "valid interception",
				request: &proto.RecordInterceptionRequest{
					Id:          uuid.NewString(),
					InitiatorId: uuid.NewString(),
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					StartedAt:   timestamppb.Now(),
				},
				setupMocks: func(db *dbmock.MockStore) {
					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Any()).Return(database.AIBridgeInterception{}, nil)
				},
			},
			{
				name: "invalid interception ID",
				request: &proto.RecordInterceptionRequest{
					Id:          "not-a-uuid",
					InitiatorId: uuid.NewString(),
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
					InitiatorId: "not-a-uuid",
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					StartedAt:   timestamppb.Now(),
				},
				expectedErr: "invalid initiator ID",
			},
			{
				name: "database error",
				request: &proto.RecordInterceptionRequest{
					Id:          uuid.NewString(),
					InitiatorId: uuid.NewString(),
					Provider:    "anthropic",
					Model:       "claude-4-opus",
					StartedAt:   timestamppb.Now(),
				},
				setupMocks: func(db *dbmock.MockStore) {
					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Any()).Return(database.AIBridgeInterception{}, sql.ErrConnDone)
				},
				expectedErr: "start interception",
			},
		},
	)
}

func TestRecordTokenUsage(t *testing.T) {
	t.Parallel()

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordTokenUsageRequest) (*proto.RecordTokenUsageResponse, error) {
			return srv.RecordTokenUsage(ctx, req)
		},
		[]struct {
			name        string
			request     *proto.RecordTokenUsageRequest
			setupMocks  func(*dbmock.MockStore)
			expectedErr string
		}{
			{
				name: "valid token usage",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					OutputTokens:   200,
					Metadata: map[string]*anypb.Any{
						"key": mustMarshalAny(t, &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "value"}}),
					},
					CreatedAt: timestamppb.Now(),
				},
				setupMocks: func(db *dbmock.MockStore) {
					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).Return(nil)
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
				name: "database error",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					OutputTokens:   200,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(db *dbmock.MockStore) {
					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
				},
				expectedErr: "insert token usage",
			},
		},
	)
}

func TestRecordPromptUsage(t *testing.T) {
	t.Parallel()

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordPromptUsageRequest) (*proto.RecordPromptUsageResponse, error) {
			return srv.RecordPromptUsage(ctx, req)
		},
		[]struct {
			name        string
			request     *proto.RecordPromptUsageRequest
			setupMocks  func(*dbmock.MockStore)
			expectedErr string
		}{
			{
				name: "valid prompt usage",
				request: &proto.RecordPromptUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					Prompt:         "yo",
					Metadata: map[string]*anypb.Any{
						"model": mustMarshalAny(t, &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "claude-4-opus"}}),
					},
					CreatedAt: timestamppb.Now(),
				},
				setupMocks: func(db *dbmock.MockStore) {
					db.EXPECT().InsertAIBridgeUserPrompt(gomock.Any(), gomock.Any()).Return(nil)
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
				setupMocks: func(db *dbmock.MockStore) {
					db.EXPECT().InsertAIBridgeUserPrompt(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
				},
				expectedErr: "insert user prompt",
			},
		},
	)
}

func TestRecordToolUsage(t *testing.T) {
	t.Parallel()

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordToolUsageRequest) (*proto.RecordToolUsageResponse, error) {
			return srv.RecordToolUsage(ctx, req)
		},
		[]struct {
			name        string
			request     *proto.RecordToolUsageRequest
			setupMocks  func(*dbmock.MockStore)
			expectedErr string
		}{
			{
				name: "valid tool usage with all fields",
				request: &proto.RecordToolUsageRequest{
					InterceptionId:  uuid.NewString(),
					MsgId:           "msg_123",
					ServerUrl:       strPtr("https://api.example.com"),
					Tool:            "read_file",
					Input:           `{"path": "/etc/hosts"}`,
					Injected:        false,
					InvocationError: strPtr("permission denied"),
					Metadata: map[string]*anypb.Any{
						"duration": mustMarshalAny(t, &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 123.45}}),
					},
					CreatedAt: timestamppb.Now(),
				},
				setupMocks: func(db *dbmock.MockStore) {
					db.EXPECT().InsertAIBridgeToolUsage(gomock.Any(), gomock.Any()).Return(nil)
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
				setupMocks: func(db *dbmock.MockStore) {
					db.EXPECT().InsertAIBridgeToolUsage(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
				},
				expectedErr: "insert tool usage",
			},
		},
	)
}

// testRecordMethod is a helper that abstracts the common testing pattern for all Record* methods
func testRecordMethod[Req any, Resp any](
	t *testing.T,
	callMethod func(*aibridgedserver.Server, context.Context, Req) (Resp, error),
	cases []struct {
		name        string
		request     Req
		setupMocks  func(*dbmock.MockStore)
		expectedErr string
	},
) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			logger := testutil.Logger(t)

			if tc.setupMocks != nil {
				tc.setupMocks(db)
			}

			srv, err := aibridgedserver.NewServer(t.Context(), db, logger, "/", nil, requiredExperiments)
			require.NoError(t, err)

			resp, err := callMethod(srv, t.Context(), tc.request)
			if tc.expectedErr != "" {
				require.Error(t, err, "Expected error for test case: %s", tc.name)
				require.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err, "Unexpected error for test case: %s", tc.name)
				require.NotNil(t, resp)
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

func strPtr(s string) *string {
	return &s
}
