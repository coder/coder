package authzquery_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (suite *MethodTestSuite) TestLicense() {
	suite.Run("GetLicenses", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
				Uuid: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			})
			require.NoError(t, err)
			return methodCase(inputs(), asserts(l, rbac.ActionRead))
		})
	})
}
