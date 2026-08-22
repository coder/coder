package entity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
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
// CredentialTypeAPIKey holds the hex of a SHA-256 digest as a password does,
// and beside it the capability the key confers: a token name, a set of scopes,
// and an allow list. It is the first credential type whose issuance takes
// parameters, and so the first to need a line in the journal.
const (
	CredentialTypePassword = "password"
	CredentialTypeNull     = "null"
	CredentialTypeAPIKey   = "api_key"
)

// Credential lifecycle events.
const (
	EventCredentialIssue  Event = "issue"
	EventCredentialRevoke Event = "revoke"
)

// Credential use events. Both name a presentation, because both are one: what
// differs is how it went.
const (
	EventPresentationAccepted Event = "presentation_accepted"
	EventPresentationRefused  Event = "presentation_refused"
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

	// APIKey carries what an api_key credential needs and no other type does.
	// It must be present exactly when Type is CredentialTypeAPIKey: absent for
	// that type there is nothing to issue, and present for another type it
	// would be a parameter to an operation that does not take one.
	APIKey *APIKeyCredential
}

// APIKeyCredential is the api_key type's own input to issuance.
//
// These are the particulars a line of the journal records, and they are also
// what the ledger holds afterwards. Those are not the same statement: the line
// says what the issuance carried, and the ledger says what the credential
// currently is.
type APIKeyCredential struct {
	// TokenName is how the credential is found and revoked. It is unique per
	// holder in the table this eventually mirrors into.
	TokenName string

	// Scopes and AllowList are the capability the key confers. They are
	// capability rather than authorization, which is a different level: see
	// "Capability becomes checkable against authorization" in
	// poc_audit/rewrite_rbac.md.
	Scopes    database.APIKeyScopes
	AllowList database.AllowList
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
	// A type's own parameters are present exactly for that type. Absent where
	// they are needed there is nothing to issue; present where they are not
	// they parameterize an operation that takes none.
	if (credentialType == CredentialTypeAPIKey) != (params.APIKey != nil) {
		return IssuedCredential{}, xerrors.Errorf(
			"credential type %q and the api_key parameters must be given together or not at all", credentialType)
	}

	switch credentialType {
	case CredentialTypePassword, CredentialTypeAPIKey:
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

	if params.APIKey != nil && len(params.APIKey.AllowList) == 0 {
		return IssuedCredential{}, xerrors.New("an api_key credential with an empty allow list confers nothing")
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

		_, err = tx.InsertCredentialLifecycleJournalEntry(ctx, database.InsertCredentialLifecycleJournalEntryParams{
			EntryID:       entryID,
			EffectiveDate: effective,
			ActorType:     string(params.Actor.Type),
			Actor:         params.Actor.ID,
			Event:         string(EventCredentialIssue),
			Subject:       issued.ID,
		})
		if err != nil {
			return xerrors.Errorf("append issuance entry: %w", err)
		}

		_, err = tx.InsertCredentialLedgerRow(ctx, database.InsertCredentialLedgerRowParams{
			ID:             issued.ID,
			HolderType:     string(params.Holder.Type),
			HolderID:       params.Holder.ID,
			CredentialType: credentialType,
			State:          CredentialStateValid,
			// Nothing issues an expiry yet. The column is here so that the
			// work package which does changes no schema, and an absent expiry
			// means no expiry: the null stands exactly where a row would have
			// been absent had expirations been kept in a table of their own.
			ExpiresAt:                 sql.NullTime{},
			LifecyclePostingReference: entryID,
		})
		if err != nil {
			return xerrors.Errorf("post to the ledger: %w", err)
		}

		// The type's own state, in the same transaction as the row it belongs
		// to. A password credential whose digest is missing is one nothing can
		// verify, so the two are written together or not at all.
		switch credentialType {
		case CredentialTypePassword:
			if _, err := tx.InsertCredentialPassword(ctx, database.InsertCredentialPasswordParams{
				ID:                  issued.ID,
				HashedAuthenticator: stored,
			}); err != nil {
				return xerrors.Errorf("post the password: %w", err)
			}
		case CredentialTypeAPIKey:
			// The line first, then the row it posts to, for the same reason
			// the entry precedes the ledger: the journal is the book of
			// original entry. Line zero, this being the only line.
			if _, err := tx.InsertCredentialLifecycleJournalAPIKeyLine(ctx, database.InsertCredentialLifecycleJournalAPIKeyLineParams{
				EntryID:   entryID,
				Line:      0,
				TokenName: params.APIKey.TokenName,
				Scopes:    params.APIKey.Scopes,
				AllowList: params.APIKey.AllowList,
			}); err != nil {
				return xerrors.Errorf("append the api_key line: %w", err)
			}
			if _, err := tx.InsertCredentialAPIKey(ctx, database.InsertCredentialAPIKeyParams{
				ID:           issued.ID,
				HashedSecret: stored,
				TokenName:    params.APIKey.TokenName,
				Scopes:       params.APIKey.Scopes,
				AllowList:    params.APIKey.AllowList,
			}); err != nil {
				return xerrors.Errorf("post the api_key: %w", err)
			}
		}
		return nil
	}, nil)
	if err != nil {
		return IssuedCredential{}, err
	}
	return issued, nil
}

// Presentation is one offering of a credential to a verifier.
//
// It carries two things and their being two is the point: the presenter
// **declares** which credential is being presented, and supplies an
// authenticator output for it. Verifying the output establishes possession; the
// declaration says what possession is being claimed of. A password style
// exchange conflates them by sending one blob, and without the declaration a
// refusal names no credential.
type Presentation struct {
	// Declared is the credential the presenter says they are presenting.
	Declared uuid.UUID

	// AuthenticatorOutput is what the presenter supplies as proof of
	// possession.
	AuthenticatorOutput string

	// Verifier is the party the presentation was made to, and so the actor of
	// whichever operation results. Both operations are observed and the
	// verifier is what noticed.
	Verifier Ref

	// AnnotationSource records where the presentation arrived from, as the
	// verifier observed it. Reliable, and an annotation because it bears on
	// nothing the operation assigns.
	//
	// There is no field for who the presenter claimed to be. Declared is the
	// only claim a presentation carries, and it is recorded as the entry's
	// subject. A field for a presenter would want a claim distinct from the
	// declaration, which arises under delegation and does not arise here.
	AnnotationSource string
}

// VerifyCredential decides one presentation and records it.
//
// The decision is whether the declared credential is valid and accepts the
// authenticator output. The record is an entry in the credential's use journal,
// posted to the two variables the use model holds, per "The credential use
// model" in poc_audit/entity_model.md.
//
// **A declared credential that does not exist is refused and not recorded.**
// There is no subject for an entry to be about. That leaves probing for
// credential identifiers untraceable here, which is a gap rather than a
// decision.
//
// **Expiry is not evaluated.** Nothing writes an expiry yet, and the clock
// check belongs to the work package that will. A credential past an expiry this
// function cannot see would be accepted.
func VerifyCredential(ctx context.Context, store database.Store, p Presentation) (bool, error) {
	if !p.Verifier.Type.Valid() || p.Verifier.ID == uuid.Nil {
		return false, xerrors.New("a presentation is observed by a verifier, so deciding one needs one")
	}

	credential, err := store.GetCredentialLedgerRowByID(ctx, p.Declared)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, xerrors.Errorf("read the declared credential: %w", err)
	}

	accepted, err := accepts(ctx, store, credential, p.AuthenticatorOutput)
	if err != nil {
		return false, err
	}

	event := EventPresentationRefused
	if accepted {
		event = EventPresentationAccepted
	}
	if err := recordPresentation(ctx, store, p, event); err != nil {
		return false, xerrors.Errorf("record the presentation: %w", err)
	}
	return accepted, nil
}

// accepts reports whether a credential accepts an authenticator output, without
// recording anything. A credential that is not valid accepts nothing, whatever
// was presented, which is checked before the comparison so that a revoked
// credential and a wrong output are refused alike.
func accepts(ctx context.Context, store database.Store, credential database.CredentialLedger, output string) (bool, error) {
	if credential.State != CredentialStateValid {
		return false, nil
	}

	digest := hashAuthenticator(output)

	switch credential.CredentialType {
	case CredentialTypeNull:
		return true, nil
	case CredentialTypeAPIKey:
		key, err := store.GetCredentialAPIKeyByID(ctx, credential.ID)
		if err != nil {
			// A ledger row of this type with no row there is a credential
			// nothing can verify, and it verifies nothing rather than erroring,
			// which is the answer a wrong output already gets.
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return false, xerrors.Errorf("read the api_key credential: %w", err)
		}
		return subtle.ConstantTimeCompare([]byte(key.HashedSecret), []byte(digest)) == 1, nil
	case CredentialTypePassword:
		password, err := store.GetCredentialPasswordByID(ctx, credential.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return false, xerrors.Errorf("read the password credential: %w", err)
		}
		return subtle.ConstantTimeCompare([]byte(password.HashedAuthenticator), []byte(digest)) == 1, nil
	default:
		// A type with no code able to validate it validates nothing. That is
		// what the absent database constraint leaves to be handled here.
		return false, nil
	}
}

// recordPresentation writes the entry and posts it.
//
// **The journal records every presentation.** That is the widest subsequence a
// predicate can select and so needs no argument about gaps, and it is a proof
// of concept cheat: the predicate is a constant here rather than state on the
// ledger row, so nothing can order that recording be narrowed or widened.
func recordPresentation(ctx context.Context, store database.Store, p Presentation, event Event) error {
	accepted := event == EventPresentationAccepted
	at := time.Now()

	return store.InTx(func(tx database.Store) error {
		entryID, err := tx.NextCredentialUseJournalEntryID(ctx)
		if err != nil {
			return xerrors.Errorf("take an entry identifier: %w", err)
		}

		if _, err := tx.InsertCredentialUseJournalEntry(ctx, database.InsertCredentialUseJournalEntryParams{
			EntryID:       entryID,
			EffectiveDate: at,
			ActorType:     string(p.Verifier.Type),
			Actor:         p.Verifier.ID,
			Event:         string(event),
			Subject:       p.Declared,
			AnnotationSource: sql.NullString{
				String: p.AnnotationSource,
				Valid:  p.AnnotationSource != "",
			},
		}); err != nil {
			return xerrors.Errorf("append the presentation entry: %w", err)
		}

		// Affecting no row means a later entry already posted, which is not a
		// failure: the fold in journal order would give that later value
		// anyway.
		if _, err := tx.PostCredentialPresentation(ctx, database.PostCredentialPresentationParams{
			ID:          p.Declared,
			PresentedAt: sql.NullTime{Time: at, Valid: true},
			Accepted:    accepted,
			EntryID:     sql.NullInt64{Int64: entryID, Valid: true},
		}); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("post the presentation: %w", err)
		}
		return nil
	}, nil)
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
		current, err := tx.GetCredentialLedgerRowByID(ctx, id)
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

		_, err = tx.InsertCredentialLifecycleJournalEntry(ctx, database.InsertCredentialLifecycleJournalEntryParams{
			EntryID:       entryID,
			EffectiveDate: time.Now(),
			ActorType:     string(actor.Type),
			Actor:         actor.ID,
			Event:         string(EventCredentialRevoke),
			Subject:       id,
		})
		if err != nil {
			return xerrors.Errorf("append revocation entry: %w", err)
		}

		if _, err := tx.RevokeCredential(ctx, database.RevokeCredentialParams{
			ID:                          id,
			LifecyclePostingReference:   entryID,
			LifecyclePostingReference_2: current.LifecyclePostingReference,
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
