package entity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/cryptorand"
)

// Credential states. See the credential lifecycle in
// poc_audit/entity_model.md. A credential is valid or it is not; there is
// nothing between.
const (
	CredentialStateValid   = "valid"
	CredentialStateInvalid = "invalid"
)

// Credential types.
//
// CredentialTypePassword holds the hex of a SHA-256 digest of the
// authenticator. The digest is unsalted because the authenticator is randomly
// generated and high entropy, which is the same reasoning coderd/apikey
// follows.
//
// CredentialTypeNull always validates and holds an empty value. It exists for
// fault isolation in tests and would never be issued in production. **The path
// that always validates is real code**, and nothing here prevents a credential
// of this type being issued outside a test. In production the type would be
// compiled out, and its presence in a ledger would then be evidence of an
// intrusion rather than of a credential.
const (
	CredentialTypePassword = "password"
	CredentialTypeNull     = "null"
)

// Credential lifecycle events.
const (
	EventCredentialIssue  Event = "issue"
	EventCredentialRevoke Event = "revoke"
)

// authenticatorLength is how many characters a minted password carries. It is
// not a considered figure.
const authenticatorLength = 32

// IssueCredentialParams are the inputs to issuing a credential.
type IssueCredentialParams struct {
	// Holder is the party the credential will authenticate.
	Holder Ref

	// Type selects what kind of credential to issue. Empty means a password.
	Type string

	// Actor is the party whose act this issuance is.
	Actor Ref

	// EffectiveAt is when the issuance happened. Zero means now.
	EffectiveAt time.Time
}

// IssuedCredential is what issuing produced.
type IssuedCredential struct {
	// ID identifies the credential, and is not derived from its secret.
	ID uuid.UUID

	// Authenticator is what the holder possesses and controls. This is the
	// only time it can be had: what the ledger keeps cannot be reversed into
	// it. For a null credential it is empty.
	Authenticator string
}

// IssueCredential issues a credential to a holder and records it.
//
// The entry is written before the ledger row it accounts for, the journal being
// the book of original entry, and the row carries the identifier of the entry
// that produced it.
//
// store may be a transaction handle, so issuance can commit with whatever else
// brought the holder into being.
func IssueCredential(ctx context.Context, store database.Store, params IssueCredentialParams) (IssuedCredential, error) {
	if !params.Holder.Type.Valid() {
		return IssuedCredential{}, xerrors.Errorf("holder type %q names no kind of entity", params.Holder.Type)
	}
	if params.Holder.ID == uuid.Nil {
		return IssuedCredential{}, xerrors.New("a credential authenticates a holder, so issuing one needs one")
	}
	if !params.Actor.Type.Valid() {
		return IssuedCredential{}, xerrors.Errorf("actor type %q names no kind of entity", params.Actor.Type)
	}
	if params.Actor.ID == uuid.Nil {
		return IssuedCredential{}, xerrors.New("an entry needs an actor, so issuance needs one")
	}

	credentialType := params.Type
	if credentialType == "" {
		credentialType = CredentialTypePassword
	}

	var issued IssuedCredential
	var stored string
	switch credentialType {
	case CredentialTypePassword:
		authenticator, err := cryptorand.String(authenticatorLength)
		if err != nil {
			return IssuedCredential{}, xerrors.Errorf("generate authenticator: %w", err)
		}
		issued.Authenticator = authenticator
		stored = hashAuthenticator(authenticator)
	case CredentialTypeNull:
		// Nothing to mint and nothing to keep. Both halves are empty on
		// purpose, and verification never consults either.
	default:
		return IssuedCredential{}, xerrors.Errorf("credential type %q has no code able to validate it", credentialType)
	}

	effective := params.EffectiveAt
	if effective.IsZero() {
		effective = time.Now()
	}

	issued.ID = uuid.New()
	err := store.InTx(func(tx database.Store) error {
		entryID, err := tx.NextCredentialLifecycleJournalEntryID(ctx)
		if err != nil {
			return xerrors.Errorf("take an entry identifier: %w", err)
		}

		_, err = tx.InsertCredentialLifecycleJournalFirstLine(ctx, database.InsertCredentialLifecycleJournalFirstLineParams{
			EntryID:       entryID,
			EffectiveDate: sql.NullTime{Time: effective, Valid: true},
			ActorType:     sql.NullString{String: string(params.Actor.Type), Valid: true},
			Actor:         uuid.NullUUID{UUID: params.Actor.ID, Valid: true},
			Event:         string(EventCredentialIssue),
			Subject:       issued.ID,
		})
		if err != nil {
			return xerrors.Errorf("append issuance entry: %w", err)
		}

		_, err = tx.InsertCredentialLifecycleLedgerRow(ctx, database.InsertCredentialLifecycleLedgerRowParams{
			ID:              issued.ID,
			HolderType:      string(params.Holder.Type),
			HolderID:        params.Holder.ID,
			CredentialType:  credentialType,
			CredentialValue: stored,
			State:           CredentialStateValid,
			// Nothing issues an expiry yet. The column is here so that the
			// work package which does changes no schema, and an absent expiry
			// means no expiry: the null stands exactly where a row would have
			// been absent had expirations been kept in a table of their own.
			ExpiresAt:        sql.NullTime{},
			PostingReference: entryID,
		})
		if err != nil {
			return xerrors.Errorf("post to the ledger: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return IssuedCredential{}, err
	}
	return issued, nil
}

// VerifyCredential reports whether an authenticator output is accepted for a
// holder.
//
// This is the validation function, and it takes an identity because validation
// always needs one. Where that identity comes from is a property of the
// presentation rather than of validation: stated outright at a first
// establishment, carried by a session, or implied by a sandbox that holds one
// AI agent. Several presentations therefore share this one function.
//
// A holder may hold several valid credentials at once, so that a rotation can
// overlap. Every candidate is compared and any match suffices. All are compared
// even once one has matched, and each comparison is constant time, so neither
// the duration nor the number of credentials is observable from outside.
//
// **Expiry is not evaluated here.** Nothing writes an expiry yet, and the clock
// check belongs to the work package that will. A credential past an expiry this
// function cannot see would verify.
func VerifyCredential(ctx context.Context, store database.Store, holder Ref, presented string) (bool, error) {
	if !holder.Type.Valid() {
		return false, xerrors.Errorf("holder type %q names no kind of entity", holder.Type)
	}

	candidates, err := store.GetValidCredentialsByHolder(ctx, database.GetValidCredentialsByHolderParams{
		HolderType: string(holder.Type),
		HolderID:   holder.ID,
	})
	if err != nil {
		return false, xerrors.Errorf("read valid credentials: %w", err)
	}

	matched := 0
	for _, candidate := range candidates {
		switch candidate.CredentialType {
		case CredentialTypeNull:
			matched = 1
		case CredentialTypePassword:
			matched |= subtle.ConstantTimeCompare(
				[]byte(candidate.CredentialValue),
				[]byte(hashAuthenticator(presented)),
			)
		default:
			// A type with no code able to validate it validates nothing. That
			// is what the absent database constraint leaves to be handled
			// here.
		}
	}

	// A holder with no valid credentials verifies nothing, including an empty
	// presented value. The loop gives that for free: nothing to match against.
	return matched == 1, nil
}

// RevokeCredential invalidates a credential and records it.
//
// The update is conditioned on the posting reference the caller read, so two
// posters racing cannot both believe they succeeded. Losing that race is
// reported as such rather than as success.
func RevokeCredential(ctx context.Context, store database.Store, id uuid.UUID, actor Ref) error {
	if !actor.Type.Valid() || actor.ID == uuid.Nil {
		return xerrors.New("an entry needs an actor, so revocation needs one")
	}

	return store.InTx(func(tx database.Store) error {
		current, err := tx.GetCredentialLifecycleLedgerRowByID(ctx, id)
		if err != nil {
			return xerrors.Errorf("read the credential: %w", err)
		}
		if current.State != CredentialStateValid {
			return xerrors.Errorf("credential %s is already %s", id, current.State)
		}

		entryID, err := tx.NextCredentialLifecycleJournalEntryID(ctx)
		if err != nil {
			return xerrors.Errorf("take an entry identifier: %w", err)
		}

		_, err = tx.InsertCredentialLifecycleJournalFirstLine(ctx, database.InsertCredentialLifecycleJournalFirstLineParams{
			EntryID:       entryID,
			EffectiveDate: sql.NullTime{Time: time.Now(), Valid: true},
			ActorType:     sql.NullString{String: string(actor.Type), Valid: true},
			Actor:         uuid.NullUUID{UUID: actor.ID, Valid: true},
			Event:         string(EventCredentialRevoke),
			Subject:       id,
		})
		if err != nil {
			return xerrors.Errorf("append revocation entry: %w", err)
		}

		if _, err := tx.RevokeCredential(ctx, database.RevokeCredentialParams{
			ID:                 id,
			PostingReference:   entryID,
			PostingReference_2: current.PostingReference,
		}); err != nil {
			return xerrors.Errorf("post the revocation: %w", err)
		}
		return nil
	}, nil)
}

// hashAuthenticator is the single place an authenticator becomes what the
// ledger keeps. Unsalted SHA-256, hex encoded, matching coderd/apikey.
func hashAuthenticator(authenticator string) string {
	sum := sha256.Sum256([]byte(authenticator))
	return hex.EncodeToString(sum[:])
}
