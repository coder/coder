package dbauthz

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// AsGitHubRunnerController authorizes runner reconciliation and workspace cleanup.
func AsGitHubRunnerController(ctx context.Context) context.Context {
	return As(ctx, rbac.Subject{
		Type: rbac.SubjectTypeGitHubRunnerController,
		ID:   uuid.Nil.String(), FriendlyName: "GitHub runner controller",
		Roles: rbac.Roles([]rbac.Role{{
			Identifier: rbac.RoleIdentifier{Name: "github-runner-controller"},
			Site: rbac.Permissions(map[string][]policy.Action{
				rbac.ResourceGithubRunnerPool.Type: rbac.ResourceGithubRunnerPool.AvailableActions(),
				rbac.ResourceWorkspace.Type:        {policy.ActionRead, policy.ActionDelete},
				rbac.ResourceTemplate.Type:         {policy.ActionRead},
				rbac.ResourceUser.Type:             {policy.ActionRead},
				rbac.ResourceProvisionerJobs.Type:  {policy.ActionRead},
			}),
		}}), Scope: rbac.ScopeAll,
	}.WithCachedASTValue())
}

func (q *querier) InsertGitHubRunnerPool(ctx context.Context, arg database.InsertGitHubRunnerPoolParams) (database.GithubRunnerPool, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceGithubRunnerPool.InOrg(arg.OrganizationID)); err != nil {
		return database.GithubRunnerPool{}, err
	}
	return q.db.InsertGitHubRunnerPool(ctx, arg)
}

func (q *querier) GetGitHubRunnerPoolByID(ctx context.Context, id uuid.UUID) (database.GithubRunnerPool, error) {
	p, err := q.db.GetGitHubRunnerPoolByID(ctx, id)
	if err != nil {
		return p, err
	}
	if err = q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceGithubRunnerPool.InOrg(p.OrganizationID).WithID(p.ID)); err != nil {
		return database.GithubRunnerPool{}, err
	}
	return p, nil
}

func (q *querier) GetGitHubRunnerPools(ctx context.Context, organizationID uuid.UUID) ([]database.GithubRunnerPool, error) {
	obj := rbac.ResourceGithubRunnerPool
	if organizationID != uuid.Nil {
		obj = obj.InOrg(organizationID)
	}
	if err := q.authorizeContext(ctx, policy.ActionRead, obj); err != nil {
		return nil, err
	}
	return q.db.GetGitHubRunnerPools(ctx, organizationID)
}

func (q *querier) authorizeGitHubRunnerPool(ctx context.Context, id uuid.UUID, action policy.Action) error {
	p, err := q.db.GetGitHubRunnerPoolByID(ctx, id)
	if err != nil {
		return err
	}
	return q.authorizeContext(ctx, action, rbac.ResourceGithubRunnerPool.InOrg(p.OrganizationID).WithID(id))
}

func (q *querier) UpdateGitHubRunnerPool(ctx context.Context, arg database.UpdateGitHubRunnerPoolParams) (database.GithubRunnerPool, error) {
	if err := q.authorizeGitHubRunnerPool(ctx, arg.ID, policy.ActionUpdate); err != nil {
		return database.GithubRunnerPool{}, err
	}
	return q.db.UpdateGitHubRunnerPool(ctx, arg)
}

func (q *querier) UpdateGitHubRunnerPoolStatus(ctx context.Context, arg database.UpdateGitHubRunnerPoolStatusParams) error {
	if err := q.authorizeGitHubRunnerPool(ctx, arg.ID, policy.ActionUpdate); err != nil {
		return err
	}
	return q.db.UpdateGitHubRunnerPoolStatus(ctx, arg)
}

func (q *querier) AcquireGitHubRunnerPoolLease(ctx context.Context, arg database.AcquireGitHubRunnerPoolLeaseParams) (uuid.UUID, error) {
	if err := q.authorizeGitHubRunnerPool(ctx, arg.ID, policy.ActionUpdate); err != nil {
		return uuid.Nil, err
	}
	return q.db.AcquireGitHubRunnerPoolLease(ctx, arg)
}

func (q *querier) ReleaseGitHubRunnerPoolLease(ctx context.Context, arg database.ReleaseGitHubRunnerPoolLeaseParams) error {
	if err := q.authorizeGitHubRunnerPool(ctx, arg.ID, policy.ActionUpdate); err != nil {
		return err
	}
	return q.db.ReleaseGitHubRunnerPoolLease(ctx, arg)
}

func (q *querier) UpdateEncryptedGitHubRunnerPoolToken(ctx context.Context, arg database.UpdateEncryptedGitHubRunnerPoolTokenParams) (database.GithubRunnerPool, error) {
	if err := q.authorizeGitHubRunnerPool(ctx, arg.ID, policy.ActionUpdate); err != nil {
		return database.GithubRunnerPool{}, err
	}
	return q.db.UpdateEncryptedGitHubRunnerPoolToken(ctx, arg)
}

func (q *querier) InsertGitHubRunner(ctx context.Context, arg database.InsertGitHubRunnerParams) (database.GithubRunner, error) {
	p, err := q.db.GetGitHubRunnerPoolByID(ctx, arg.PoolID)
	if err != nil {
		return database.GithubRunner{}, err
	}
	if p.OrganizationID != arg.OrganizationID {
		return database.GithubRunner{}, NotAuthorizedError{}
	}
	if err = q.authorizeGitHubRunnerPool(ctx, arg.PoolID, policy.ActionUpdate); err != nil {
		return database.GithubRunner{}, err
	}
	return q.db.InsertGitHubRunner(ctx, arg)
}

func (q *querier) GetGitHubRunnersByPoolID(ctx context.Context, id uuid.UUID) ([]database.GithubRunner, error) {
	if err := q.authorizeGitHubRunnerPool(ctx, id, policy.ActionRead); err != nil {
		return nil, err
	}
	return q.db.GetGitHubRunnersByPoolID(ctx, id)
}

func (q *querier) UpdateGitHubRunner(ctx context.Context, arg database.UpdateGitHubRunnerParams) (database.GithubRunner, error) {
	// Runner identity is immutable. Resolve its pool before authorizing updates.
	r, err := q.db.GetGitHubRunnerByID(ctx, arg.ID)
	if err != nil {
		return database.GithubRunner{}, err
	}
	if err = q.authorizeGitHubRunnerPool(ctx, r.PoolID, policy.ActionUpdate); err != nil {
		return database.GithubRunner{}, err
	}
	return q.db.UpdateGitHubRunner(ctx, arg)
}

func (q *querier) GetGitHubRunnerByID(ctx context.Context, id uuid.UUID) (database.GithubRunner, error) {
	r, err := q.db.GetGitHubRunnerByID(ctx, id)
	if err != nil {
		return r, err
	}
	if err = q.authorizeGitHubRunnerPool(ctx, r.PoolID, policy.ActionRead); err != nil {
		return database.GithubRunner{}, err
	}
	return r, nil
}

// CancelGitHubRunnerWorkspaceBuild cancels only builds belonging to an allocation.
func (q *querier) CancelGitHubRunnerWorkspaceBuild(ctx context.Context, arg database.CancelGitHubRunnerWorkspaceBuildParams) error {
	r, err := q.db.GetGitHubRunnerByID(ctx, arg.RunnerID)
	if err != nil {
		return err
	}
	if err = q.authorizeGitHubRunnerPool(ctx, r.PoolID, policy.ActionUpdate); err != nil {
		return err
	}
	return q.db.CancelGitHubRunnerWorkspaceBuild(ctx, arg)
}
