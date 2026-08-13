package entity

import (
	"context"
	"crypto/subtle"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/cryptorand"
)

// credentialLength is how many characters a minted password carries. It is not
// a considered figure. Nothing about the placeholder credential is.
const credentialLength = 32

// IssueCredential mints a credential for an entity and records it as valid.
// The credential is returned, and this is the only time it can be: nothing
// reads it back out for a caller.
//
// Every credential here is a password. That is a proof of concept
// simplification, not a position on what credentials should be.
//
// store may be a transaction handle, so issuance can commit with whatever
// else brought the entity into being.
func IssueCredential(ctx context.Context, store database.Store, holder Ref) (string, error) {
	if !holder.Type.Valid() {
		return "", xerrors.Errorf("holder type %q names no kind of entity", holder.Type)
	}

	password, err := cryptorand.String(credentialLength)
	if err != nil {
		return "", xerrors.Errorf("generate credential: %w", err)
	}

	if _, err := store.InsertValidCredential(ctx, database.InsertValidCredentialParams{
		ActorType: string(holder.Type),
		Actor:     holder.ID,
		Password:  password,
	}); err != nil {
		return "", xerrors.Errorf("record credential as valid: %w", err)
	}

	return password, nil
}

// VerifyCredential reports whether presented is one of the credentials
// currently valid for holder.
//
// An entity may hold several at once while a rotation overlaps, so every
// candidate is compared and any match suffices. All of them are compared even
// once one has matched, and each comparison is constant time, so that neither
// how long verification took nor how many credentials exist is observable from
// outside.
//
// A credential that has been revoked is not here to be found. Revocation
// deletes the row, and the account of it belongs to the journal.
func VerifyCredential(ctx context.Context, store database.Store, holder Ref, presented string) (bool, error) {
	if !holder.Type.Valid() {
		return false, xerrors.Errorf("holder type %q names no kind of entity", holder.Type)
	}

	valid, err := store.GetValidCredentialsByActor(ctx, database.GetValidCredentialsByActorParams{
		ActorType: string(holder.Type),
		Actor:     holder.ID,
	})
	if err != nil {
		return false, xerrors.Errorf("read valid credentials: %w", err)
	}

	matched := 0
	for _, candidate := range valid {
		matched |= subtle.ConstantTimeCompare([]byte(candidate.Password), []byte(presented))
	}

	// An entity with no valid credentials verifies nothing, including an empty
	// presented value. The loop gives that for free: nothing to match against.
	return matched == 1, nil
}
