package dbcrypt

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

func (db *dbCrypt) InsertGitHubRunnerPool(ctx context.Context, arg database.InsertGitHubRunnerPoolParams) (database.GithubRunnerPool, error) {
	if err := db.encryptField(&arg.GithubToken, &arg.GithubTokenKeyID); err != nil {
		return database.GithubRunnerPool{}, err
	}
	p, err := db.Store.InsertGitHubRunnerPool(ctx, arg)
	if err != nil {
		return p, err
	}
	err = db.decryptField(&p.GithubToken, p.GithubTokenKeyID)
	return p, err
}

func (db *dbCrypt) GetGitHubRunnerPoolByID(ctx context.Context, id uuid.UUID) (database.GithubRunnerPool, error) {
	p, err := db.Store.GetGitHubRunnerPoolByID(ctx, id)
	if err != nil {
		return p, err
	}
	err = db.decryptField(&p.GithubToken, p.GithubTokenKeyID)
	return p, err
}

func (db *dbCrypt) GetGitHubRunnerPools(ctx context.Context, organization uuid.UUID) ([]database.GithubRunnerPool, error) {
	pools, err := db.Store.GetGitHubRunnerPools(ctx, organization)
	if err != nil {
		return nil, err
	}
	for i := range pools {
		if err = db.decryptField(&pools[i].GithubToken, pools[i].GithubTokenKeyID); err != nil {
			return nil, err
		}
	}
	return pools, nil
}

func (db *dbCrypt) UpdateGitHubRunnerPool(ctx context.Context, arg database.UpdateGitHubRunnerPoolParams) (database.GithubRunnerPool, error) {
	p, err := db.Store.UpdateGitHubRunnerPool(ctx, arg)
	if err != nil {
		return p, err
	}
	err = db.decryptField(&p.GithubToken, p.GithubTokenKeyID)
	return p, err
}

func (db *dbCrypt) UpdateEncryptedGitHubRunnerPoolToken(ctx context.Context, arg database.UpdateEncryptedGitHubRunnerPoolTokenParams) (database.GithubRunnerPool, error) {
	if err := db.encryptField(&arg.GithubToken, &arg.GithubTokenKeyID); err != nil {
		return database.GithubRunnerPool{}, err
	}
	p, err := db.Store.UpdateEncryptedGitHubRunnerPoolToken(ctx, arg)
	if err != nil {
		return p, err
	}
	err = db.decryptField(&p.GithubToken, p.GithubTokenKeyID)
	return p, err
}
