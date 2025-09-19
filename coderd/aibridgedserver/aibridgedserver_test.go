package aibridgedserver_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/aibridgedserver"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

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

			srv, err := aibridgedserver.NewServer(t.Context(), db, logger, "/", nil)
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
