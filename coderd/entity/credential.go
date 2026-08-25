package entity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
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
//
// `revoke` is commanded and carries the party that withdrew the credential.
// `lapse` and `discharge` are entailed and carry no actor: a lapse arises when
// the credential's holder ceases to exist, and a discharge when the thing the
// credential was accessory to ends while the holder does not. Nobody decides
// either. See "The credential lifecycle" in poc_audit/entity_model.md.
const (
	EventCredentialIssue     Event = "issue"
	EventCredentialRevoke    Event = "revoke"
	EventCredentialLapse     Event = "lapse"
	EventCredentialDischarge Event = "discharge"
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

// The shape of an api_key token, which is "<key id>-<secret>" at exactly these
// lengths. httpmw.SplitAPIToken parses nothing else, and it is not imported
// here because it would draw the middleware into this package; the acceptance
// test in poc_tests is what catches the two drifting apart.
//
// A credential type is therefore not only what the ledger holds for it but
// what shape its authenticator takes, because the authenticator has to be
// readable by whatever verifies it.
const (
	apiKeyIDLength     = 10
	apiKeySecretLength = 22
)

// apiKeyMirrorNoExpiry is the expiry the mirror writes for a credential the
// ledger records no expiry for.
//
// **api_keys.expires_at is NOT NULL and so cannot say "never".** Every api key
// that existed before this had been issued by a login and expired with the
// session, so the case never arose. A credential held by an AI agent has no
// session to outlive and ends by revocation instead. The mirror represents that
// as faithfully as a narrower column allows, which is to say by a date chosen
// to be recognizable rather than by a fact.
var apiKeyMirrorNoExpiry = time.Date(9999, time.January, 1, 0, 0, 0, 0, time.UTC)

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

	// MirrorLifetime is how long the mirrored api_keys row says the credential
	// lasts. Zero means the mirror's stand-in for never.
	//
	// **A cheat, and named to say so.** The ledger records no expiry for a
	// credential, expiry having been raised and deliberately left unsettled.
	// This field is read by the mirror and by nothing else: it is not folded,
	// not journaled, and no ledger row holds it. It exists so that an issuer
	// which already expires its keys keeps doing so when it is routed through
	// here, rather than being silently converted into one that does not.
	//
	// **It preserves function until expiry gets a proper treatment, which
	// supersedes it.** When the model holds expiry, the fact moves to the
	// ledger and this field goes.
	MirrorLifetime time.Duration
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
// preparedIssuance is everything an issuance settles before it reaches the
// database: what was minted, what will be kept of it, and which shapes the
// writes take. Preparing it apart from the transaction keeps generation and
// validation off the connection, and lets a rotation issue on a line of an
// entry it did not open.
type preparedIssuance struct {
	params         IssueCredentialParams
	credentialType string
	issued         IssuedCredential
	stored         string
	keyID          string
	keySecret      string
	mirrorHolder   database.HolderType
	effective      time.Time
}

func prepareIssuance(params IssueCredentialParams) (preparedIssuance, error) {
	if !params.Holder.Type.Valid() {
		return preparedIssuance{}, xerrors.Errorf("holder type %q names no kind of entity", params.Holder.Type)
	}
	if params.Holder.ID == uuid.Nil {
		return preparedIssuance{}, xerrors.New("a credential authenticates a holder, so issuing one needs one")
	}
	if !params.Actor.Type.Valid() {
		return preparedIssuance{}, xerrors.Errorf("actor type %q names no kind of entity", params.Actor.Type)
	}
	if params.Actor.ID == uuid.Nil {
		return preparedIssuance{}, xerrors.New("an entry needs an actor, so issuance needs one")
	}

	credentialType := params.Type
	if credentialType == "" {
		credentialType = CredentialTypePassword
	}

	var issued IssuedCredential
	var stored string
	// The two halves of an api_key token, empty for every other type. The
	// mirror needs both after the switch, so they outlive the case.
	var keyID, keySecret string
	// A type's own parameters are present exactly for that type. Absent where
	// they are needed there is nothing to issue; present where they are not
	// they parameterize an operation that takes none.
	if (credentialType == CredentialTypeAPIKey) != (params.APIKey != nil) {
		return preparedIssuance{}, xerrors.Errorf(
			"credential type %q and the api_key parameters must be given together or not at all", credentialType)
	}

	switch credentialType {
	case CredentialTypePassword:
		authenticator, err := cryptorand.String(authenticatorLength)
		if err != nil {
			return preparedIssuance{}, xerrors.Errorf("generate authenticator: %w", err)
		}
		issued.Authenticator = authenticator
		stored = hashAuthenticator(authenticator)
	case CredentialTypeAPIKey:
		id, err := cryptorand.String(apiKeyIDLength)
		if err != nil {
			return preparedIssuance{}, xerrors.Errorf("generate a key id: %w", err)
		}
		secret, err := cryptorand.String(apiKeySecretLength)
		if err != nil {
			return preparedIssuance{}, xerrors.Errorf("generate a key secret: %w", err)
		}
		keyID, keySecret = id, secret
		// What is handed to the holder packs a declaration and an
		// authenticator output into one string, and what is kept is a digest
		// of the second half alone. Splitting the token is the verifier's
		// work, so a presentation of this credential carries the secret half
		// as its authenticator output and the credential's own identifier as
		// its declaration.
		issued.Authenticator = id + "-" + secret
		stored = hashAuthenticator(secret)
	case CredentialTypeNull:
		// Nothing to mint and nothing to keep. Both halves are empty on
		// purpose, and verification never consults either.
	default:
		return preparedIssuance{}, xerrors.Errorf("credential type %q has no code able to validate it", credentialType)
	}

	if params.APIKey != nil && len(params.APIKey.AllowList) == 0 {
		return preparedIssuance{}, xerrors.New("an api_key credential with an empty allow list confers nothing")
	}

	// The mirror narrows what may hold a credential. api_keys constrains its
	// holder to a user or an AI agent, so a credential for a workspace_agent
	// can be recorded in the ledger and cannot be mirrored. Saying so here
	// names the restriction; letting it through would report it as a
	// constraint violation on a table the caller never mentioned.
	var mirrorHolder database.HolderType
	if credentialType == CredentialTypeAPIKey {
		var err error
		mirrorHolder, err = apiKeyHolderType(params.Holder.Type)
		if err != nil {
			return preparedIssuance{}, err
		}
	}

	effective := params.EffectiveAt
	if effective.IsZero() {
		effective = time.Now()
	}

	issued.ID = uuid.New()
	return preparedIssuance{
		params:         params,
		credentialType: credentialType,
		issued:         issued,
		stored:         stored,
		keyID:          keyID,
		keySecret:      keySecret,
		mirrorHolder:   mirrorHolder,
		effective:      effective,
	}, nil
}

// appendCommandedEntry writes the entry a commanded operation occupies. It
// carries the party and the moment and nothing about a subject, which is the
// line's, and it names nothing as having entailed it because a party acted.
func appendCommandedEntry(ctx context.Context, tx database.Store, entryID int64, effective time.Time, actor Ref) error {
	_, err := tx.InsertCredentialLifecycleJournalEntry(ctx, database.InsertCredentialLifecycleJournalEntryParams{
		EntryID:              entryID,
		EffectiveDate:        effective,
		ActorType:            sql.NullString{String: string(actor.Type), Valid: true},
		Actor:                uuid.NullUUID{UUID: actor.ID, Valid: true},
		EntailedByEntry:      sql.NullInt64{},
		EntailedByAnnotation: sql.NullString{},
	})
	if err != nil {
		return xerrors.Errorf("append entry: %w", err)
	}
	return nil
}

func IssueCredential(ctx context.Context, store database.Store, params IssueCredentialParams) (IssuedCredential, error) {
	p, err := prepareIssuance(params)
	if err != nil {
		return IssuedCredential{}, err
	}

	err = store.InTx(func(tx database.Store) error {
		entryID, err := tx.NextCredentialLifecycleJournalEntryID(ctx)
		if err != nil {
			return xerrors.Errorf("take an entry identifier: %w", err)
		}
		if err := appendCommandedEntry(ctx, tx, entryID, p.effective, p.params.Actor); err != nil {
			return err
		}
		// Line zero, an issuance on its own naming one credential.
		return postIssuance(ctx, tx, entryID, 0, p)
	}, nil)
	if err != nil {
		return IssuedCredential{}, err
	}
	return p.issued, nil
}

// postIssuance writes one issuance as a line of an entry already opened, and
// posts everything that line implies: the ledger row, the type's own state, and
// the mirror.
//
// **It does not open the entry**, which is what lets a rotation put an issuance
// and a revocation into one. Its caller supplies the entry and the line number.
func postIssuance(ctx context.Context, tx database.Store, entryID int64, line int16, p preparedIssuance) error {
	params, credentialType, issued := p.params, p.credentialType, p.issued
	stored, keyID, keySecret := p.stored, p.keyID, p.keySecret
	mirrorHolder, effective := p.mirrorHolder, p.effective

	// The subject and what happened to it are the line's, the entry carrying
	// only who acted and when.
	if _, err := tx.InsertCredentialLifecycleJournalLine(ctx, database.InsertCredentialLifecycleJournalLineParams{
		EntryID: entryID,
		Line:    line,
		Subject: issued.ID,
		Event:   string(EventCredentialIssue),
	}); err != nil {
		return xerrors.Errorf("append the issuance line: %w", err)
	}

	if _, err := tx.InsertCredentialLedgerRow(ctx, database.InsertCredentialLedgerRowParams{
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
	}); err != nil {
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
		// original entry. The same line the issuance took, this saying
		// what that line carried.
		if _, err := tx.InsertCredentialLifecycleJournalAPIKeyLine(ctx, database.InsertCredentialLifecycleJournalAPIKeyLineParams{
			EntryID:   entryID,
			Line:      line,
			TokenName: params.APIKey.TokenName,
			Scopes:    params.APIKey.Scopes,
			AllowList: params.APIKey.AllowList,
		}); err != nil {
			return xerrors.Errorf("append the api_key line: %w", err)
		}
		if _, err := tx.InsertCredentialAPIKey(ctx, database.InsertCredentialAPIKeyParams{
			ID:           issued.ID,
			KeyID:        keyID,
			HashedSecret: stored,
			TokenName:    params.APIKey.TokenName,
			Scopes:       params.APIKey.Scopes,
			AllowList:    params.APIKey.AllowList,
		}); err != nil {
			return xerrors.Errorf("post the api_key: %w", err)
		}

		mirrorExpiry := apiKeyMirrorNoExpiry
		if params.APIKey.MirrorLifetime > 0 {
			mirrorExpiry = effective.Add(params.APIKey.MirrorLifetime)
		}

		// The mirror, in the transaction that posted the credential.
		// api_keys is what authenticates a request today, so a credential
		// the ledger holds and that table does not is one the system will
		// refuse. Writing both together is what makes issuance through the
		// journal the same act as issuance, rather than a description of
		// one that happened elsewhere.
		//
		// **This is one way and only for issuance.** Revocation, expiry
		// and last use still write api_keys directly, so the two can
		// diverge on every path but this one, and nothing detects it.
		if _, err := tx.InsertAPIKey(ctx, database.InsertAPIKeyParams{
			ID:              keyID,
			HashedSecret:    hashAuthenticatorBytes(keySecret),
			HolderID:        database.HolderID(params.Holder.ID),
			HolderType:      mirrorHolder,
			LastUsed:        time.Unix(0, 0).UTC(),
			ExpiresAt:       mirrorExpiry,
			LifetimeSeconds: int64(mirrorExpiry.Sub(effective).Seconds()),
			CreatedAt:       effective,
			UpdatedAt:       effective,
			// A key minted on request rather than obtained by logging in.
			// This is also what the unique index on (holder_id,
			// token_name) is conditioned on, so a holder cannot be issued
			// two credentials of the same name.
			LoginType: database.LoginTypeToken,
			IPAddress: pqtype.Inet{
				IPNet: net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(32, 32)},
				Valid: true,
			},
			Scopes:    params.APIKey.Scopes,
			AllowList: params.APIKey.AllowList,
			TokenName: params.APIKey.TokenName,
		}); err != nil {
			return xerrors.Errorf("mirror into api_keys: %w", err)
		}
	}
	return nil
}

// RotateCredential issues a credential and revokes the one it replaces, as a
// single entry naming both.
//
// **This is what the atomic group is for.** entity_model.md holds that a
// rotation is issuing one credential and revoking another, one entry with two
// subjects, and that recording it as two entries would assert the very gap the
// overlap exists to prevent. One party, one moment, two lines.
//
// **Line zero revokes and line one issues, and the order is the mirror's
// doing.** api_keys carries a unique index over a holder and a token name for
// minted tokens, so the superseded row goes before the replacement arrives.
// Inside one transaction that ordering is invisible to every other reader. It
// constrains the writes and not the model: the ledger holds both credentials,
// and this goes when the mirror does.
//
// The superseded credential must be valid. Rotating an ended credential is not
// a rotation, and issuing a replacement for one is an issuance.
func RotateCredential(ctx context.Context, store database.Store, superseded uuid.UUID, params IssueCredentialParams) (IssuedCredential, error) {
	if superseded == uuid.Nil {
		return IssuedCredential{}, xerrors.New("a rotation replaces a credential, so it needs one")
	}

	p, err := prepareIssuance(params)
	if err != nil {
		return IssuedCredential{}, err
	}

	err = store.InTx(func(tx database.Store) error {
		current, err := tx.GetCredentialLedgerRowByID(ctx, superseded)
		if err != nil {
			return xerrors.Errorf("read the superseded credential: %w", err)
		}
		if current.State != CredentialStateValid {
			return xerrors.Errorf("credential %s is already %s", superseded, current.State)
		}

		entryID, err := tx.NextCredentialLifecycleJournalEntryID(ctx)
		if err != nil {
			return xerrors.Errorf("take an entry identifier: %w", err)
		}
		if err := appendCommandedEntry(ctx, tx, entryID, p.effective, p.params.Actor); err != nil {
			return err
		}

		if err := postInvalidation(ctx, tx, entryID, 0, superseded, EventCredentialRevoke, current); err != nil {
			return err
		}
		if err := dropMirror(ctx, tx, superseded); err != nil {
			return err
		}
		return postIssuance(ctx, tx, entryID, 1, p)
	}, nil)
	if err != nil {
		return IssuedCredential{}, err
	}
	return p.issued, nil
}

// dropMirror deletes the api_keys row standing for a credential, where it has
// one. Only a rotation needs this here: issuance writes the mirror, and every
// other ending still deletes it from outside, which is the divergence recorded
// on the mirror write in postIssuance.
func dropMirror(ctx context.Context, tx database.Store, credentialID uuid.UUID) error {
	mirrored, err := tx.GetCredentialAPIKeyByID(ctx, credentialID)
	if errors.Is(err, sql.ErrNoRows) {
		// Not an api_key credential, so nothing mirrors it.
		return nil
	}
	if err != nil {
		return xerrors.Errorf("read the superseded credential's key id: %w", err)
	}
	if err := tx.DeleteAPIKeyByID(ctx, mirrored.KeyID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("delete the superseded mirror: %w", err)
	}
	return nil
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

// APIKeyPresentation reads an api key token as a presentation.
//
// The token packs a declaration and an authenticator output into one string.
// This resolves the first into the credential it names and keeps the second
// unchanged. Resolving is the verifier's act rather than the presenter's: what
// the presenter supplied was a key id, which is what the wire has instead of an
// identifier.
//
// A token naming no credential yields a presentation declaring none, which
// VerifyCredential refuses without recording, for the reason given there. A
// token that is not a token at all is an error, there being no presentation to
// build from it.
func APIKeyPresentation(ctx context.Context, store database.Store, token string, verifier Ref, source string) (Presentation, error) {
	keyID, secret, ok := strings.Cut(token, "-")
	if !ok || len(keyID) != apiKeyIDLength || len(secret) != apiKeySecretLength {
		return Presentation{}, xerrors.New("an api key token is a key id and a secret joined by a hyphen")
	}

	p := Presentation{
		AuthenticatorOutput: secret,
		Verifier:            verifier,
		AnnotationSource:    source,
	}

	key, err := store.GetCredentialAPIKeyByKeyID(ctx, keyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return p, nil
		}
		return Presentation{}, xerrors.Errorf("resolve the declared key id: %w", err)
	}
	p.Declared = key.ID
	return p, nil
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

// optionalActor renders an actor for a journal entry, absent for an entailed
// operation. The null stands exactly where a normalized form would have had no
// row, per "The actor column is nullable, and null there means there was no
// actor" in poc_audit/implementation_patterns.md.
func optionalActor(actor Ref) (sql.NullString, uuid.NullUUID) {
	if actor.Type == "" || actor.ID == uuid.Nil {
		return sql.NullString{}, uuid.NullUUID{}
	}
	return sql.NullString{String: string(actor.Type), Valid: true},
		uuid.NullUUID{UUID: actor.ID, Valid: true}
}

// EntailedBy says what entailed an operation, in one of the two forms an
// entailed entry may take.
//
// **Exactly one field is set.** Entry names the entry the operation followed
// from, which is available where the thing that entailed it keeps a journal.
// Annotation says in words what entailed it, for where that thing keeps none.
// See "The reference has two forms, and one of them is words" in
// poc_audit/implementation_patterns.md.
//
// **Using the annotation is a standing policy and each use of it is
// transitory.** It is replaced by a reference once its referent is modeled and
// journaled.
type EntailedBy struct {
	Entry      int64
	Annotation string
}

// Valid reports whether exactly one form is present.
func (e EntailedBy) Valid() bool {
	return (e.Entry != 0) != (e.Annotation != "")
}

// DischargeCredential invalidates a credential because the thing it was
// accessory to has ended, and records it.
//
// **Entailed, so there is no actor.** Nobody withdrew the credential and nobody
// noticed it become pointless; it follows from an ending the record already
// holds. See "How the credential machine is read" in poc_audit/entity_model.md
// for the grounds, of which there are four.
//
// **This transition conflates those four**, which is permitted while the model
// is being made and is recorded as outstanding. What distinguishes them today
// is the annotation.
//
// store may be a transaction handle, so a discharge can commit with the ending
// that caused it.
func DischargeCredential(ctx context.Context, store database.Store, id uuid.UUID, entailedBy EntailedBy, effectiveAt time.Time) error {
	if !entailedBy.Valid() {
		return xerrors.New("a discharge says what entailed it, by entry or in words, and never both")
	}

	return invalidateCredential(ctx, store, id, Ref{}, EventCredentialDischarge, effectiveAt, entailedBy)
}

// RevokeCredential invalidates a credential deliberately and records it.
//
// **Commanded.** Some party withdrew the credential, whether because it is
// suspected, superseded, or no longer wanted, and the actor is that party.
// LapseCredential is the observed counterpart, reaching the same state for a
// reason nobody chose.
func RevokeCredential(ctx context.Context, store database.Store, id uuid.UUID, actor Ref) error {
	if !actor.Type.Valid() || actor.ID == uuid.Nil {
		return xerrors.New("an entry needs an actor, so revocation needs one")
	}

	return invalidateCredential(ctx, store, id, actor, EventCredentialRevoke, time.Time{}, EntailedBy{})
}

// LapseCredential invalidates a credential because what it rested on went
// away, and records it.
//
// **Status: this signature is wrong and a rework is planned.** It was written
// when a lapse was classed observed, so it takes an actor and refuses to run
// without one, and callers pass SystemActor to satisfy it. A lapse is now held
// to be entailed, and an entailed operation has no actor at all. What is here
// is therefore a cheat sustaining a cheat: a fixed system identity, filed among
// users because there is nowhere else to put one, standing in for a party that
// does not exist.
//
// The rework removes the actor rather than finding a better one, and takes the
// same decision for `discharge`, which is entailed on the same grounds and must
// not acquire this shape by being written to match. Until then, read the actor
// on a lapse entry as noise.
//
// store may be a transaction handle, so a lapse can commit with the ending that
// caused it.
func LapseCredential(ctx context.Context, store database.Store, id uuid.UUID, actor Ref, effectiveAt time.Time) error {
	if !actor.Type.Valid() || actor.ID == uuid.Nil {
		return xerrors.New("an entry needs an actor, so a lapse needs one")
	}

	return invalidateCredential(ctx, store, id, actor, EventCredentialLapse, effectiveAt, EntailedBy{})
}

// invalidateCredential writes the entry and posts it. Both transitions into
// `invalid` come through here, differing only in the event they record and in
// where their actor came from, so the posting is written once.
//
// The update is conditioned on the posting reference the caller read, so two
// posters racing cannot both believe they succeeded. Losing that race is
// reported as such rather than as success.
func invalidateCredential(ctx context.Context, store database.Store, id uuid.UUID, actor Ref, event Event, effectiveAt time.Time, entailedBy EntailedBy) error {
	effective := effectiveAt
	if effective.IsZero() {
		effective = time.Now()
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

		actorType, actorID := optionalActor(actor)
		_, err = tx.InsertCredentialLifecycleJournalEntry(ctx, database.InsertCredentialLifecycleJournalEntryParams{
			EntryID:              entryID,
			EffectiveDate:        effective,
			ActorType:            actorType,
			Actor:                actorID,
			EntailedByEntry:      sql.NullInt64{Int64: entailedBy.Entry, Valid: entailedBy.Entry != 0},
			EntailedByAnnotation: sql.NullString{String: entailedBy.Annotation, Valid: entailedBy.Annotation != ""},
		})
		if err != nil {
			return xerrors.Errorf("append %s entry: %w", event, err)
		}

		// Line zero, an ending naming the one credential it ends. A retirement
		// ending several of a holder's credentials still writes an entry
		// apiece; gathering those into one entry with a line each is what this
		// shape now permits and is not done here.
		return postInvalidation(ctx, tx, entryID, 0, id, event, current)
	}, nil)
}

// postInvalidation writes one ending as a line of an entry already opened, and
// posts it against the ledger row the line names.
//
// **It does not open the entry**, for the reason postIssuance does not: a
// rotation revokes on a line of the entry that issues on another.
//
// The posting is conditioned on the reference the caller read, so two posters
// racing cannot both believe they succeeded.
func postInvalidation(ctx context.Context, tx database.Store, entryID int64, line int16, id uuid.UUID, event Event, current database.CredentialLedger) error {
	if _, err := tx.InsertCredentialLifecycleJournalLine(ctx, database.InsertCredentialLifecycleJournalLineParams{
		EntryID: entryID,
		Line:    line,
		Subject: id,
		Event:   string(event),
	}); err != nil {
		return xerrors.Errorf("append the %s line: %w", event, err)
	}

	if _, err := tx.InvalidateCredential(ctx, database.InvalidateCredentialParams{
		ID:                          id,
		LifecyclePostingReference:   entryID,
		LifecyclePostingReference_2: current.LifecyclePostingReference,
	}); err != nil {
		return xerrors.Errorf("post the %s: %w", event, err)
	}
	return nil
}

// hashAuthenticator is the single place an authenticator becomes what the
// ledger keeps. Unsalted SHA-256, hex encoded, matching coderd/apikey.
func hashAuthenticator(authenticator string) string {
	sum := sha256.Sum256([]byte(authenticator))
	return hex.EncodeToString(sum[:])
}

// hashAuthenticatorBytes is the same digest api_keys holds. The ledger keeps
// hex and that table keeps bytes, so the two columns differ in encoding and
// agree in content.
func hashAuthenticatorBytes(authenticator string) []byte {
	sum := sha256.Sum256([]byte(authenticator))
	return sum[:]
}

// apiKeyHolderType maps a holder onto what api_keys accepts, and reports the
// kinds it does not.
func apiKeyHolderType(t Type) (database.HolderType, error) {
	switch t {
	case TypeUser:
		return database.HolderTypeUser, nil
	case TypeAIAgent:
		return database.HolderTypeAIAgent, nil
	default:
		return "", xerrors.Errorf("api_keys holds no credential for a %s", t)
	}
}
