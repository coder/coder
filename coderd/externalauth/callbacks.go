package externalauth

import (
	"context"
	"fmt"
	"net/url"

	"github.com/google/go-github/v61/github"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

type githubCallback struct {
	accessURL *url.URL
	db        database.Store
}

func (c githubCallback) LinkPublicKey(ctx context.Context, userID uuid.UUID, token string) error {
	client := github.NewClient(nil).WithAuthToken(token)

	ghKeys, _, err := client.Users.ListKeys(ctx, "", nil)
	if err != nil {
		return xerrors.Errorf("list github keys: %w", err)
	}

	dbKey, err := c.db.GetGitSSHKey(ctx, userID)
	if err != nil {
		return xerrors.Errorf("get git ssh key: %w", err)
	}

	for _, key := range ghKeys {
		if key.GetKey() == dbKey.PublicKey {
			return nil
		}
	}

	_, _, err = client.Users.CreateKey(ctx, &github.Key{
		Key:   &dbKey.PublicKey,
		Title: github.String(fmt.Sprintf("%s Workspaces", c.accessURL.String())),
	})
	if err != nil {
		return xerrors.Errorf("create github key: %w", err)
	}

	return nil
}
