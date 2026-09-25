package chatd

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
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
)

// RBAC authorizes only the intersection of these scopes and the owner's current
// permissions. Including unrestricted use here does not grant that privilege.
var syntheticAPIKeyScopes = database.APIKeyScopes{
	database.ApiKeyScopeApiKeyRead,
	database.ApiKeyScopeChatModelConfigUse,
	database.ApiKeyScopeAIGatewayUnrestrictedUse,
}

// GatewayTokenName returns the deterministic token name of the synthetic
// gateway key for a user. The name is the lookup key: no mapping table exists,
// so attribution resolves the key by (user_id, token_name, login_type !=
// 'token').
func GatewayTokenName(ownerID uuid.UUID) string {
	return fmt.Sprintf("chatd_%s_session_token", ownerID)
}

// ensureSyntheticAPIKeyID returns the ID of the synthetic gateway key for the
// given user, minting or extending it as needed. The key ID is stable for the
// lifetime of the user: near-expiry keys are extended in place rather than
// replaced, because an in-flight generation may have already delegated the
// current key ID to the gateway.
func (p *Server) ensureSyntheticAPIKeyID(ctx context.Context, ownerID uuid.UUID) (string, error) {
	ctx = dbauthz.AsChatdKeyMinter(ctx, ownerID)
	key, err := p.db.GetChatGatewayAPIKey(ctx, database.GetChatGatewayAPIKeyParams{
		UserID:    ownerID,
		TokenName: GatewayTokenName(ownerID),
	})
	if err == nil && slices.Equal(key.Scopes, syntheticAPIKeyScopes) && key.ExpiresAt.After(p.clock.Now().Add(syntheticAPIKeyRenewMargin)) {
		return key.ID, nil
	}
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return "", xerrors.Errorf("get synthetic API key: %w", err)
	}
	return p.mintSyntheticAPIKey(ctx, ownerID)
}

// mintSyntheticAPIKey extends or mints the synthetic gateway key under a
// per-user advisory lock. The lock serializes concurrent mints because the
// partial unique index on token names only covers login_type 'token' rows, so
// nothing else prevents duplicate synthetic keys.
func (p *Server) mintSyntheticAPIKey(ctx context.Context, ownerID uuid.UUID) (string, error) {
	tokenName := GatewayTokenName(ownerID)
	var keyID string
	err := p.db.InTx(func(tx database.Store) error {
		err := tx.AcquireLock(ctx, database.GenLockID("chatd_gateway_key:"+ownerID.String()))
		if err != nil {
			return xerrors.Errorf("acquire chat gateway key lock: %w", err)
		}
		key, err := tx.GetChatGatewayAPIKey(ctx, database.GetChatGatewayAPIKeyParams{
			UserID:    ownerID,
			TokenName: tokenName,
		})
		if err == nil {
			if !slices.Equal(key.Scopes, syntheticAPIKeyScopes) {
				// An in-flight transport may already hold this ID. Upgrade
				// scopes in place without changing credentials or expiry.
				key, err = tx.UpdateChatGatewayAPIKeyScopesByID(ctx, database.UpdateChatGatewayAPIKeyScopesByIDParams{
					ID:     key.ID,
					UserID: ownerID,
					Scopes: syntheticAPIKeyScopes,
				})
				if err != nil {
					return xerrors.Errorf("update synthetic API key scopes: %w", err)
				}
			}
			keyID = key.ID
			if key.ExpiresAt.After(p.clock.Now().Add(syntheticAPIKeyRenewMargin)) {
				return nil
			}
			err = tx.UpdateAPIKeyByID(ctx, database.UpdateAPIKeyByIDParams{
				ID:        key.ID,
				LastUsed:  key.LastUsed,
				ExpiresAt: p.clock.Now().Add(syntheticAPIKeyLifetime),
				IPAddress: key.IPAddress,
			})
			if err != nil {
				return xerrors.Errorf("extend synthetic API key: %w", err)
			}
			return nil
		}
		if !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("get synthetic API key: %w", err)
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
			TokenName:       tokenName,
			// The secret is discarded; delegated Gateway requests use the ID.
			// Scopes permit model use, still constrained by the owner's roles.
			Scopes: syntheticAPIKeyScopes,
		})
		if err != nil {
			return xerrors.Errorf("generate synthetic API key: %w", err)
		}
		inserted, err := tx.InsertAPIKey(ctx, params)
		if err != nil {
			return xerrors.Errorf("insert synthetic API key: %w", err)
		}
		keyID = inserted.ID
		return nil
	}, nil)
	if err != nil {
		return "", err
	}
	return keyID, nil
}
