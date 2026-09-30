package workspaceartifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Bundler collects the immutable result paths from a workspace agent.
type Bundler interface {
	BundleFiles(context.Context, workspacesdk.BundleFilesRequest) ([]byte, error)
}

type declarations struct {
	ResultPaths       []string   `json:"result_paths"`
	ArtifactExpiresAt *time.Time `json:"artifact_expires_at"`
}

// EffectiveExpiry returns an explicit recovery extension when present, otherwise
// the original declaration. A null value means no configured expiry.
func EffectiveExpiry(session database.WorkspaceExecutionSession) (sql.NullTime, error) {
	var declaration declarations
	if err := json.Unmarshal(session.Declarations, &declaration); err != nil {
		return sql.NullTime{}, xerrors.Errorf("decode result declarations: %w", err)
	}
	if session.RecoveryArtifactExpiresAt.Valid {
		return session.RecoveryArtifactExpiresAt, nil
	}
	if declaration.ArtifactExpiresAt != nil {
		return sql.NullTime{Time: declaration.ArtifactExpiresAt.UTC(), Valid: true}, nil
	}
	return sql.NullTime{}, nil
}

// Preserve validates complete agent results before atomically storing them.
// The caller must first claim the session's preserving revision. Collection
// happens outside the transaction; the workspace lifecycle lock fences the
// final revision check and inserts against cleanup. Committed results are
// verified and returned without recollecting bytes on retries.
func Preserve(ctx context.Context, db database.Store, agent Bundler, expected database.WorkspaceExecutionSession, now time.Time) ([]database.GetWorkspaceExecutionArtifactsBySessionIDRow, error) {
	if expected.State != "preserving" || !expected.WorkspaceID.Valid {
		return nil, xerrors.New("session is not preserving a workspace")
	}
	var declaration declarations
	if err := json.Unmarshal(expected.Declarations, &declaration); err != nil {
		return nil, xerrors.Errorf("decode result declarations: %w", err)
	}
	expiry, err := EffectiveExpiry(expected)
	if err != nil {
		return nil, err
	}
	if expiry.Valid && !expiry.Time.After(now) {
		return nil, xerrors.New("effective artifact expiry has elapsed; retry with an explicit future artifact_expires_at")
	}
	var result []database.GetWorkspaceExecutionArtifactsBySessionIDRow
	readExisting := func(tx database.Store) error {
		if err := tx.AcquireLock(ctx, database.WorkspaceLifecycleLockID(expected.WorkspaceID.UUID)); err != nil {
			return xerrors.Errorf("lock workspace lifecycle: %w", err)
		}
		current, err := tx.GetWorkspaceExecutionSessionByID(ctx, expected.ID)
		if err != nil {
			return xerrors.Errorf("read preserving session: %w", err)
		}
		if current.State != "preserving" || current.Revision != expected.Revision ||
			current.WorkspaceID != expected.WorkspaceID || current.WorkspaceOwnerID != expected.WorkspaceOwnerID ||
			current.OrganizationID != expected.OrganizationID || current.OwnerID != expected.OwnerID ||
			current.RecoveryArtifactExpiresAt != expected.RecoveryArtifactExpiresAt ||
			!bytes.Equal(current.Declarations, expected.Declarations) {
			return xerrors.New("session changed while collecting results")
		}
		existing, err := tx.GetWorkspaceExecutionArtifactsBySessionID(ctx, expected.ID)
		if err != nil {
			return xerrors.Errorf("read preserved results: %w", err)
		}
		result = nil
		// A nonempty revision is committed as one complete validated set. Never
		// recollect or overwrite that set, including after a lost commit response.
		for _, artifact := range existing {
			if artifact.PreservationRevision != expected.Revision {
				continue
			}
			if artifact.ExpiresAt.Valid && !artifact.ExpiresAt.Time.After(now) {
				return xerrors.New("preserved result is no longer available")
			}
			stored, err := tx.ReadWorkspaceExecutionArtifact(ctx, database.ReadWorkspaceExecutionArtifactParams{ID: artifact.ID, ByteLimit: MaxBundleBytes})
			if err != nil {
				return xerrors.Errorf("verify preserved result: %w", err)
			}
			digest := sha256.Sum256(stored.Data)
			if len(stored.Data) != int(artifact.SizeBytes) || !bytes.Equal(digest[:], artifact.Sha256) {
				return xerrors.New("preserved result failed integrity verification")
			}
			result = append(result, artifact)
		}
		return nil
	}
	if err := db.InTx(readExisting, nil); err != nil {
		return nil, err
	}
	if len(result) > 0 || len(declaration.ResultPaths) == 0 {
		return result, nil
	}
	bundle, err := agent.BundleFiles(ctx, workspacesdk.BundleFilesRequest{Paths: declaration.ResultPaths})
	if err != nil {
		return nil, xerrors.Errorf("collect declared results: %w", err)
	}
	files, err := ValidateBundle(bundle, declaration.ResultPaths)
	if err != nil {
		return nil, xerrors.Errorf("validate declared results: %w", err)
	}
	err = db.InTx(func(tx database.Store) error {
		if err := readExisting(tx); err != nil {
			return err
		}
		if len(result) > 0 {
			return nil
		}
		for _, file := range files {
			digest, err := hex.DecodeString(file.SHA256)
			if err != nil {
				return xerrors.Errorf("decode result checksum: %w", err)
			}
			inserted, err := tx.InsertWorkspaceExecutionArtifact(ctx, database.InsertWorkspaceExecutionArtifactParams{
				ID: uuid.New(), SessionID: expected.ID, PreservationRevision: expected.Revision,
				SourcePath: file.Path, Name: file.Name, Mimetype: file.MIMEType, SizeBytes: int64(len(file.Data)),
				Sha256: digest, Data: file.Data, CreatedAt: now, ExpiresAt: expiry,
			})
			if err != nil {
				return xerrors.Errorf("store result %q: %w", file.Path, err)
			}
			if inserted != 1 {
				return xerrors.New("session no longer accepts preserved results")
			}
		}
		// Restrict the receipt to this preserving revision.
		return readExisting(tx)
	}, nil)
	if err != nil {
		return nil, err
	}
	return result, nil
}
