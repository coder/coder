package chatd

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

// maxArtifactVersionSize matches the attach_file limit.
const maxArtifactVersionSize = 10 << 20

// errStaleArtifactBase marks a refused version insert inside the publish
// transaction. Publish turns it into a chattool.StaleBaseError.
var errStaleArtifactBase = xerrors.New("stale artifact base version")

// artifactStore stores artifacts for one chat. A chat may use an artifact
// in its own organization that the chat owner owns or that belongs to a
// project the chat owner can use. Role grants do not count, as for project
// memory, so the store runs as chatd and checks access itself.
type artifactStore struct {
	db   database.Store
	chat database.Chat
}

var _ chattool.ArtifactStore = (*artifactStore)(nil)

func newArtifactStore(db database.Store, chat database.Chat) *artifactStore {
	return &artifactStore{db: db, chat: chat}
}

// Publish stores the next version of an artifact, or creates the artifact
// when params.ArtifactID is unset. A base version that is not the latest
// returns a *chattool.StaleBaseError.
func (s *artifactStore) Publish(ctx context.Context, params chattool.PublishArtifactParams) (chattool.PublishedArtifact, error) {
	//nolint:gocritic // The store checks the chat owner's access itself.
	ctx = dbauthz.AsChatd(ctx)
	if len(params.Data) == 0 {
		return chattool.PublishedArtifact{}, xerrors.New("artifact is empty")
	}
	if len(params.Data) > maxArtifactVersionSize {
		return chattool.PublishedArtifact{}, xerrors.Errorf("artifact exceeds %d MiB size limit", maxArtifactVersionSize>>20)
	}
	name, mediaType, err := chatfiles.PrepareStoredFile(params.Name, params.Name, params.Data)
	if err != nil {
		return chattool.PublishedArtifact{}, err
	}

	var artifact database.ChatArtifact
	var projectID uuid.NullUUID
	if params.ArtifactID.Valid {
		if params.BaseVersion <= 0 {
			return chattool.PublishedArtifact{}, xerrors.New("base_version is required with artifact_id")
		}
		artifact, err = s.usableArtifact(ctx, params.ArtifactID.UUID)
		if err != nil {
			return chattool.PublishedArtifact{}, err
		}
	} else {
		if params.BaseVersion != 0 {
			return chattool.PublishedArtifact{}, xerrors.New("base_version needs artifact_id")
		}
		if params.Title == "" {
			return chattool.PublishedArtifact{}, xerrors.New("title is required to create an artifact")
		}
		projectID, err = s.usableChatProjectID(ctx)
		if err != nil {
			return chattool.PublishedArtifact{}, err
		}
	}

	var published chattool.PublishedArtifact
	err = s.db.InTx(func(tx database.Store) error {
		if !params.ArtifactID.Valid {
			artifact, err = tx.InsertChatArtifact(ctx, database.InsertChatArtifactParams{
				ID:             uuid.NullUUID{},
				OrganizationID: s.chat.OrganizationID,
				OwnerID:        s.chat.OwnerID,
				ProjectID:      projectID,
				Title:          params.Title,
			})
			if err != nil {
				return xerrors.Errorf("insert chat artifact: %w", err)
			}
		}
		version, err := tx.InsertChatArtifactVersion(ctx, database.InsertChatArtifactVersionParams{
			ArtifactID:  artifact.ID,
			BaseVersion: params.BaseVersion,
			Label:       params.Label,
			Name:        name,
			Mimetype:    mediaType,
			Data:        params.Data,
			CreatedBy:   uuid.NullUUID{UUID: s.chat.OwnerID, Valid: true},
			ChatID:      uuid.NullUUID{UUID: s.chat.ID, Valid: true},
		})
		// No row: the base is not the latest. Key conflict: another
		// publish on the same base committed first.
		if errors.Is(err, sql.ErrNoRows) || database.IsUniqueViolation(err, database.UniqueChatArtifactVersionsPkey) {
			return errStaleArtifactBase
		}
		if err != nil {
			return xerrors.Errorf("insert chat artifact version: %w", err)
		}
		if version.Version != params.BaseVersion+1 {
			return xerrors.Errorf("inserted version %d on base %d", version.Version, params.BaseVersion)
		}
		updated, err := tx.UpdateChatArtifactOnPublish(ctx, database.UpdateChatArtifactOnPublishParams{
			ID:    artifact.ID,
			Title: params.Title,
		})
		if err != nil {
			return xerrors.Errorf("update chat artifact: %w", err)
		}
		published = chattool.PublishedArtifact{
			ArtifactID: updated.ID,
			Version:    version.Version,
			Title:      updated.Title,
			Name:       name,
		}
		return nil
	}, database.DefaultTXOptions().WithID("publish_chat_artifact"))
	if errors.Is(err, errStaleArtifactBase) {
		row, getErr := s.db.GetChatArtifactByID(ctx, artifact.ID)
		if getErr != nil {
			return chattool.PublishedArtifact{}, xerrors.Errorf("get latest artifact version: %w", getErr)
		}
		return chattool.PublishedArtifact{}, &chattool.StaleBaseError{Latest: row.LatestVersion}
	}
	if err != nil {
		return chattool.PublishedArtifact{}, err
	}
	return published, nil
}

// Read returns one version of an artifact, or the latest when version is 0.
func (s *artifactStore) Read(ctx context.Context, artifactID uuid.UUID, version int32) (chattool.ArtifactContent, error) {
	//nolint:gocritic // The store checks the chat owner's access itself.
	ctx = dbauthz.AsChatd(ctx)
	if version < 0 {
		return chattool.ArtifactContent{}, xerrors.New("version must be positive")
	}
	artifact, err := s.usableArtifact(ctx, artifactID)
	if err != nil {
		return chattool.ArtifactContent{}, err
	}
	var row database.ChatArtifactVersion
	if version == 0 {
		row, err = s.db.GetLatestChatArtifactVersion(ctx, artifact.ID)
	} else {
		row, err = s.db.GetChatArtifactVersion(ctx, database.GetChatArtifactVersionParams{ArtifactID: artifact.ID, Version: version})
	}
	if errors.Is(err, sql.ErrNoRows) {
		return chattool.ArtifactContent{}, xerrors.Errorf("artifact has no version %d", version)
	}
	if err != nil {
		return chattool.ArtifactContent{}, xerrors.Errorf("get artifact version: %w", err)
	}
	return chattool.ArtifactContent{
		ArtifactID: artifact.ID,
		Version:    row.Version,
		Title:      artifact.Title,
		Name:       row.Name,
		Data:       row.Data,
		Untrusted:  !row.CreatedBy.Valid || row.CreatedBy.UUID != s.chat.OwnerID,
	}, nil
}

// usableArtifact loads an artifact that this chat may use. Every refusal
// returns chattool.ErrArtifactNotFound, so a chat cannot probe for IDs.
func (s *artifactStore) usableArtifact(ctx context.Context, artifactID uuid.UUID) (database.ChatArtifact, error) {
	row, err := s.db.GetChatArtifactByID(ctx, artifactID)
	if errors.Is(err, sql.ErrNoRows) {
		return database.ChatArtifact{}, chattool.ErrArtifactNotFound
	}
	if err != nil {
		return database.ChatArtifact{}, xerrors.Errorf("get chat artifact: %w", err)
	}
	artifact := row.ChatArtifact
	if artifact.OrganizationID != s.chat.OrganizationID {
		return database.ChatArtifact{}, chattool.ErrArtifactNotFound
	}
	if artifact.OwnerID == s.chat.OwnerID {
		return artifact, nil
	}
	if !artifact.ProjectID.Valid {
		return database.ChatArtifact{}, chattool.ErrArtifactNotFound
	}
	project, err := s.db.GetChatProjectByID(ctx, artifact.ProjectID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		return database.ChatArtifact{}, chattool.ErrArtifactNotFound
	}
	if err != nil {
		return database.ChatArtifact{}, xerrors.Errorf("get chat artifact project: %w", err)
	}
	usable, err := ChatProjectUsableBy(ctx, s.db, project, s.chat.OwnerID)
	if err != nil {
		return database.ChatArtifact{}, err
	}
	if !usable {
		return database.ChatArtifact{}, chattool.ErrArtifactNotFound
	}
	return artifact, nil
}

// usableChatProjectID returns the chat's project when the chat owner can
// use it, so that a new artifact is visible to the project members.
func (s *artifactStore) usableChatProjectID(ctx context.Context) (uuid.NullUUID, error) {
	if !s.chat.ProjectID.Valid {
		return uuid.NullUUID{}, nil
	}
	project, err := s.db.GetChatProjectByID(ctx, s.chat.ProjectID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.NullUUID{}, nil
	}
	if err != nil {
		return uuid.NullUUID{}, xerrors.Errorf("get chat project: %w", err)
	}
	if project.OrganizationID != s.chat.OrganizationID {
		return uuid.NullUUID{}, nil
	}
	usable, err := ChatProjectUsableBy(ctx, s.db, project, s.chat.OwnerID)
	if err != nil {
		return uuid.NullUUID{}, err
	}
	if !usable {
		return uuid.NullUUID{}, nil
	}
	return uuid.NullUUID{UUID: project.ID, Valid: true}, nil
}
