package aibridgedserver_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	protobufproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	codermcp "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/x/aibridged"
	"github.com/coder/coder/v2/enterprise/x/aibridged/proto"
	"github.com/coder/coder/v2/enterprise/x/aibridgedserver"
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
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(database.User{ID: user.ID, Deleted: true}, nil)
			},
		},
		{
			name:        "system user",
			expectedErr: aibridgedserver.ErrSystemUser,
			mocksFn: func(db *dbmock.MockStore, apiKey database.APIKey, user database.User) {
				db.EXPECT().GetAPIKeyByID(gomock.Any(), apiKey.ID).Times(1).Return(apiKey, nil)
				db.EXPECT().GetUserByID(gomock.Any(), user.ID).Times(1).Return(database.User{ID: user.ID, IsSystem: true}, nil)
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
			keySecret, _ := cryptorand.String(22)
			token := fmt.Sprintf("%s-%s", keyID, keySecret)
			keySecretHashed := sha256.Sum256([]byte(keySecret))
			apiKey := database.APIKey{
				ID:              keyID,
				LifetimeSeconds: 86400, // default in db
				HashedSecret:    keySecretHashed[:],
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

			accessURL := "https://my-cool-deployment.com"
			srv, err := aibridgedserver.NewServer(t.Context(), db, logger, accessURL, tc.externalAuthConfigs, tc.experiments)
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
					Id:          uuid.NewString(),
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
						ID:          interceptionID,
						InitiatorID: initiatorID,
						Provider:    req.GetProvider(),
						Model:       req.GetModel(),
						Metadata:    json.RawMessage(metadataJSON),
						StartedAt:   req.StartedAt.AsTime().UTC(),
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
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordInterceptionRequest) {
					db.EXPECT().InsertAIBridgeInterception(gomock.Any(), gomock.Any()).Return(database.AIBridgeInterception{}, sql.ErrConnDone)
				},
				expectedErr: "start interception",
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
	)

	testRecordMethod(t,
		func(srv *aibridgedserver.Server, ctx context.Context, req *proto.RecordTokenUsageRequest) (*proto.RecordTokenUsageResponse, error) {
			return srv.RecordTokenUsage(ctx, req)
		},
		[]testRecordMethodCase[*proto.RecordTokenUsageRequest]{
			{
				name: "valid token usage",
				request: &proto.RecordTokenUsageRequest{
					InterceptionId: uuid.NewString(),
					MsgId:          "msg_123",
					InputTokens:    100,
					OutputTokens:   200,
					Metadata:       metadataProto,
					CreatedAt:      timestamppb.Now(),
				},
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					interceptionID, err := uuid.Parse(req.GetInterceptionId())
					assert.NoError(t, err, "parse interception UUID")

					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Cond(func(p database.InsertAIBridgeTokenUsageParams) bool {
						if !assert.NotEqual(t, uuid.Nil, p.ID, "ID") ||
							!assert.Equal(t, interceptionID, p.InterceptionID, "interception ID") ||
							!assert.Equal(t, req.GetMsgId(), p.ProviderResponseID, "provider response ID") ||
							!assert.Equal(t, req.GetInputTokens(), p.InputTokens, "input tokens") ||
							!assert.Equal(t, req.GetOutputTokens(), p.OutputTokens, "output tokens") ||
							!assert.JSONEq(t, metadataJSON, string(p.Metadata), "metadata") ||
							!assert.WithinDuration(t, req.GetCreatedAt().AsTime(), p.CreatedAt, time.Second, "created at") {
							return false
						}
						return true
					})).Return(database.AIBridgeTokenUsage{
						ID:                 uuid.New(),
						InterceptionID:     interceptionID,
						ProviderResponseID: req.GetMsgId(),
						InputTokens:        req.GetInputTokens(),
						OutputTokens:       req.GetOutputTokens(),
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
				setupMocks: func(t *testing.T, db *dbmock.MockStore, req *proto.RecordTokenUsageRequest) {
					db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), gomock.Any()).Return(database.AIBridgeTokenUsage{}, sql.ErrConnDone)
				},
				expectedErr: "insert token usage",
			},
		},
	)
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
					ServerUrl:       strPtr("https://api.example.com"),
					Tool:            "read_file",
					Input:           `{"path": "/etc/hosts"}`,
					Injected:        false,
					InvocationError: strPtr("permission denied"),
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

type testRecordMethodCase[Req any] struct {
	name    string
	request Req
	// setupMocks is called with the mock store and the above request.
	setupMocks  func(t *testing.T, db *dbmock.MockStore, req Req)
	expectedErr string
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
			srv, err := aibridgedserver.NewServer(ctx, db, logger, "/", nil, requiredExperiments)
			require.NoError(t, err)

			resp, err := callMethod(srv, ctx, tc.request)
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
