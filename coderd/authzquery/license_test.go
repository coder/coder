package authzquery_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

func (s *MethodTestSuite) TestLicense() {
	s.Run("GetLicenses", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
				Uuid: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			})
			require.NoError(t, err)
			return methodCase(values(), asserts(l, rbac.ActionRead),
				values([]database.License{l}))
		})
	})
	s.Run("InsertLicense", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(database.InsertLicenseParams{}),
				asserts(rbac.ResourceLicense, rbac.ActionCreate), nil)
		})
	})
	s.Run("InsertOrUpdateLogoURL", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(rbac.ResourceDeploymentConfig, rbac.ActionCreate), nil)
		})
	})
	s.Run("InsertOrUpdateServiceBanner", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values("value"), asserts(rbac.ResourceDeploymentConfig, rbac.ActionCreate), nil)
		})
	})
	s.Run("GetLicenseByID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
				Uuid: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			})
			require.NoError(t, err)
			return methodCase(values(l.ID), asserts(l, rbac.ActionRead), values(l))
		})
	})
	s.Run("DeleteLicense", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
				Uuid: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			})
			require.NoError(t, err)
			return methodCase(values(l.ID), asserts(l, rbac.ActionDelete), nil)
		})
	})
	s.Run("GetDeploymentID", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(), values(""))
		})
	})
	s.Run("GetLogoURL", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			err := db.InsertOrUpdateLogoURL(context.Background(), "value")
			require.NoError(t, err)
			return methodCase(values(), asserts(), values("value"))
		})
	})
	s.Run("GetServiceBanner", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			err := db.InsertOrUpdateServiceBanner(context.Background(), "value")
			require.NoError(t, err)
			return methodCase(values(), asserts(), values("value"))
		})
	})
}
