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
			return methodCase(values(), asserts(l, rbac.ActionRead),
				values([]database.License{l}))
		})
	})
	suite.Run("InsertLicense", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertLicenseParams{}),
				asserts(rbac.ResourceLicense, rbac.ActionCreate), nil)
		})
	})
	suite.Run("InsertOrUpdateLogoURL", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(rbac.ResourceDeploymentConfig, rbac.ActionUpdate), nil)
		})
	})
	suite.Run("InsertOrUpdateServiceBanner", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(rbac.ResourceDeploymentConfig, rbac.ActionUpdate), nil)
		})
	})
	suite.Run("GetLicenseByID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
				Uuid: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			})
			require.NoError(t, err)
			return methodCase(values(l.ID), asserts(l, rbac.ActionRead), values(l))
		})
	})
	suite.Run("DeleteLicense", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
				Uuid: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			})
			require.NoError(t, err)
			return methodCase(values(l.ID), asserts(l, rbac.ActionDelete), nil)
		})
	})
	suite.Run("GetDeploymentID", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), values(""))
		})
	})
	suite.Run("GetLogoURL", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			err := db.InsertOrUpdateLogoURL(context.Background(), "value")
			require.NoError(t, err)
			return methodCase(values(), asserts(), values("value"))
		})
	})
	suite.Run("GetServiceBanner", func() {
		suite.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			err := db.InsertOrUpdateServiceBanner(context.Background(), "value")
			require.NoError(t, err)
			return methodCase(values(), asserts(), values("value"))
		})
	})
}
