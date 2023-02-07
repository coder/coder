package authzquery_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestLicense() {
	s.Run("GetLicenses", s.Subtest(func(db database.Store, check *MethodCase) {
		l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Uuid: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		})
		require.NoError(s.T(), err)
		check.Args().Asserts(l, rbac.ActionRead).
			Returns([]database.License{l})
	}))
	s.Run("InsertLicense", s.Subtest(func(db database.Store, check *MethodCase) {
		check.Args(database.InsertLicenseParams{}).
			Asserts(rbac.ResourceLicense, rbac.ActionCreate)
	}))
	s.Run("InsertOrUpdateLogoURL", s.Subtest(func(db database.Store, check *MethodCase) {
		check.Args("value").Asserts(rbac.ResourceDeploymentConfig, rbac.ActionCreate)
	}))
	s.Run("InsertOrUpdateServiceBanner", s.Subtest(func(db database.Store, check *MethodCase) {
		check.Args("value").Asserts(rbac.ResourceDeploymentConfig, rbac.ActionCreate)
	}))
	s.Run("GetLicenseByID", s.Subtest(func(db database.Store, check *MethodCase) {
		l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Uuid: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		})
		require.NoError(s.T(), err)
		check.Args(l.ID).Asserts(l, rbac.ActionRead).Returns(l)
	}))
	s.Run("DeleteLicense", s.Subtest(func(db database.Store, check *MethodCase) {
		l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			Uuid: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		})
		require.NoError(s.T(), err)
		check.Args(l.ID).Asserts(l, rbac.ActionDelete)
	}))
	s.Run("GetDeploymentID", s.Subtest(func(db database.Store, check *MethodCase) {
		check.Args().Asserts().Returns("")
	}))
	s.Run("GetLogoURL", s.Subtest(func(db database.Store, check *MethodCase) {
		err := db.InsertOrUpdateLogoURL(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts().Returns("value")
	}))
	s.Run("GetServiceBanner", s.Subtest(func(db database.Store, check *MethodCase) {
		err := db.InsertOrUpdateServiceBanner(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts().Returns("value")
	}))
}
