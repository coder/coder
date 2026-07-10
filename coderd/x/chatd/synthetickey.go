package chatd

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

const (
	syntheticAPIKeyLifetime    = 30 * 24 * time.Hour
	syntheticAPIKeyRenewMargin = 24 * time.Hour
	syntheticAPIKeyMaxAttempts = 3
)

func (p *Server) ensureSyntheticAPIKeyID(ctx context.Context, ownerID uuid.UUID) (string, error) {
	ctx = dbauthz.AsChatdKeyMinter(ctx, ownerID)
	for range syntheticAPIKeyMaxAttempts {
		mapping, err := p.db.GetChatSyntheticAPIKeyByUserID(ctx, ownerID)
		if err == nil {
			key, keyErr := p.db.GetAPIKeyByID(ctx, mapping.APIKeyID)
			switch {
			case keyErr == nil && key.ExpiresAt.After(p.clock.Now().Add(syntheticAPIKeyRenewMargin)):
				return key.ID, nil
			case keyErr != nil && !xerrors.Is(keyErr, sql.ErrNoRows):
				return "", xerrors.Errorf("get synthetic API key: %w", keyErr)
			}
		} else if !xerrors.Is(err, sql.ErrNoRows) {
			return "", xerrors.Errorf("get synthetic API key mapping: %w", err)
		}

		keyID, retry, err := p.mintSyntheticAPIKey(ctx, ownerID)
		if err != nil {
			return "", err
		}
		if !retry {
			return keyID, nil
		}
	}
	return "", xerrors.New("ensure synthetic API key: concurrent update retry limit reached")
}

func (p *Server) mintSyntheticAPIKey(ctx context.Context, ownerID uuid.UUID) (keyID string, retry bool, err error) {
	err = p.db.InTx(func(tx database.Store) error {
		mapping, mappingErr := tx.GetChatSyntheticAPIKeyByUserID(ctx, ownerID)
		hasMapping := mappingErr == nil
		if mappingErr != nil && !xerrors.Is(mappingErr, sql.ErrNoRows) {
			return xerrors.Errorf("get synthetic API key mapping: %w", mappingErr)
		}
		if hasMapping {
			key, keyErr := tx.GetAPIKeyByID(ctx, mapping.APIKeyID)
			if keyErr == nil && key.ExpiresAt.After(p.clock.Now().Add(syntheticAPIKeyRenewMargin)) {
				keyID = key.ID
				return nil
			}
			if keyErr != nil {
				if xerrors.Is(keyErr, sql.ErrNoRows) {
					retry = true
					return nil
				}
				return xerrors.Errorf("get synthetic API key: %w", keyErr)
			}
		}

		owner, err := tx.GetUserForChatSyntheticAPIKeyByID(ctx, ownerID)
		if err != nil {
			return xerrors.Errorf("get synthetic API key owner: %w", err)
		}
		params, _, err := apikey.Generate(apikey.CreateParams{
			UserID:          ownerID,
			LoginType:       owner.LoginType,
			ExpiresAt:       p.clock.Now().Add(syntheticAPIKeyLifetime),
			LifetimeSeconds: int64(syntheticAPIKeyLifetime.Seconds()),
			TokenName:       fmt.Sprintf("%s_chat_gateway_key", ownerID),
		})
		if err != nil {
			return xerrors.Errorf("generate synthetic API key: %w", err)
		}
		key, err := tx.InsertAPIKey(ctx, params)
		if err != nil {
			return xerrors.Errorf("insert synthetic API key: %w", err)
		}

		var rows int64
		if hasMapping {
			rows, err = tx.UpdateChatSyntheticAPIKey(ctx, database.UpdateChatSyntheticAPIKeyParams{
				UserID:      ownerID,
				OldApiKeyID: mapping.APIKeyID,
				NewApiKeyID: key.ID,
			})
		} else {
			rows, err = tx.InsertChatSyntheticAPIKey(ctx, database.InsertChatSyntheticAPIKeyParams{
				UserID:   ownerID,
				APIKeyID: key.ID,
			})
		}
		if err != nil {
			return xerrors.Errorf("publish synthetic API key mapping: %w", err)
		}
		if rows == 1 {
			keyID = key.ID
			return nil
		}
		if err := tx.DeleteAPIKeyByID(ctx, key.ID); err != nil {
			return xerrors.Errorf("delete losing synthetic API key: %w", err)
		}
		retry = true
		return nil
	}, nil)
	if err != nil {
		return "", false, err
	}
	return keyID, retry, nil
}
