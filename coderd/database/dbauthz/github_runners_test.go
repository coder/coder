package dbauthz_test

import (
	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

func (s *MethodTestSuite) TestGitHubRunners() {
	s.Run("GetGitHubRunnerPoolByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})

		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		check.Args(pool.ID).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionRead).Returns(pool)
	}))
	s.Run("GetGitHubRunnerPools", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})

		dbm.EXPECT().GetGitHubRunnerPools(gomock.Any(), pool.OrganizationID).Return([]database.GithubRunnerPool{pool}, nil).AnyTimes()
		check.Args(pool.OrganizationID).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID), policy.ActionRead).Returns([]database.GithubRunnerPool{pool})
	}))
	s.Run("InsertGitHubRunnerPool", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		arg := database.InsertGitHubRunnerPoolParams{ID: pool.ID, OrganizationID: pool.OrganizationID}
		dbm.EXPECT().InsertGitHubRunnerPool(gomock.Any(), arg).Return(pool, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID), policy.ActionCreate).Returns(pool)
	}))
	s.Run("UpdateGitHubRunnerPool", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		arg := database.UpdateGitHubRunnerPoolParams{ID: pool.ID}
		dbm.EXPECT().UpdateGitHubRunnerPool(gomock.Any(), arg).Return(pool, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionUpdate).Returns(pool)
	}))
	s.Run("UpdateGitHubRunnerPoolStatus", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		arg := database.UpdateGitHubRunnerPoolStatusParams{ID: pool.ID}
		dbm.EXPECT().UpdateGitHubRunnerPoolStatus(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionUpdate).Returns()
	}))
	s.Run("AcquireGitHubRunnerPoolLease", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		arg := database.AcquireGitHubRunnerPoolLeaseParams{ID: pool.ID}
		dbm.EXPECT().AcquireGitHubRunnerPoolLease(gomock.Any(), arg).Return(pool.ID, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionUpdate).Returns(pool.ID)
	}))
	s.Run("ReleaseGitHubRunnerPoolLease", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		arg := database.ReleaseGitHubRunnerPoolLeaseParams{ID: pool.ID}
		dbm.EXPECT().ReleaseGitHubRunnerPoolLease(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateEncryptedGitHubRunnerPoolToken", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		arg := database.UpdateEncryptedGitHubRunnerPoolTokenParams{ID: pool.ID}
		dbm.EXPECT().UpdateEncryptedGitHubRunnerPoolToken(gomock.Any(), arg).Return(pool, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionUpdate).Returns(pool)
	}))
	s.Run("InsertGitHubRunner", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		runner := testutil.Fake(s.T(), faker, database.GithubRunner{PoolID: pool.ID, OrganizationID: pool.OrganizationID})
		arg := database.InsertGitHubRunnerParams{ID: runner.ID, PoolID: pool.ID, OrganizationID: pool.OrganizationID}
		dbm.EXPECT().InsertGitHubRunner(gomock.Any(), arg).Return(runner, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionUpdate).Returns(runner)
	}))
	s.Run("GetGitHubRunnersByPoolID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		runner := testutil.Fake(s.T(), faker, database.GithubRunner{PoolID: pool.ID, OrganizationID: pool.OrganizationID})

		dbm.EXPECT().GetGitHubRunnersByPoolID(gomock.Any(), pool.ID).Return([]database.GithubRunner{runner}, nil).AnyTimes()
		check.Args(pool.ID).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionRead).Returns([]database.GithubRunner{runner})
	}))
	s.Run("UpdateGitHubRunner", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		runner := testutil.Fake(s.T(), faker, database.GithubRunner{PoolID: pool.ID, OrganizationID: pool.OrganizationID})
		arg := database.UpdateGitHubRunnerParams{ID: runner.ID}
		dbm.EXPECT().GetGitHubRunnerByID(gomock.Any(), runner.ID).Return(runner, nil).AnyTimes()
		dbm.EXPECT().UpdateGitHubRunner(gomock.Any(), arg).Return(runner, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionUpdate).Returns(runner)
	}))
	s.Run("GetGitHubRunnerByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		runner := testutil.Fake(s.T(), faker, database.GithubRunner{PoolID: pool.ID, OrganizationID: pool.OrganizationID})

		dbm.EXPECT().GetGitHubRunnerByID(gomock.Any(), runner.ID).Return(runner, nil).AnyTimes()
		check.Args(runner.ID).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionRead).Returns(runner)
	}))
	s.Run("GetGitHubRunnerPools", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetGitHubRunnerPools(gomock.Any(), uuid.Nil).Return([]database.GithubRunnerPool{}, nil).AnyTimes()
		check.Args(uuid.Nil).Asserts(rbac.ResourceGithubRunnerPool, policy.ActionRead).Returns([]database.GithubRunnerPool{})
	}))
	s.Run("CancelGitHubRunnerWorkspaceBuild", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		pool := testutil.Fake(s.T(), faker, database.GithubRunnerPool{})
		runner := testutil.Fake(s.T(), faker, database.GithubRunner{PoolID: pool.ID})
		arg := database.CancelGitHubRunnerWorkspaceBuildParams{RunnerID: runner.ID, JobID: uuid.New()}
		dbm.EXPECT().GetGitHubRunnerByID(gomock.Any(), runner.ID).Return(runner, nil).AnyTimes()
		dbm.EXPECT().GetGitHubRunnerPoolByID(gomock.Any(), pool.ID).Return(pool, nil).AnyTimes()
		dbm.EXPECT().CancelGitHubRunnerWorkspaceBuild(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGithubRunnerPool.InOrg(pool.OrganizationID).WithID(pool.ID), policy.ActionUpdate).Returns()
	}))
}
