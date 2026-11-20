package idpsync

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// GroupSnapshotReader reads the GroupSnapshot that ValidityFindings needs:
// the state of the organization orgID, and the status of each UUID in
// groupIDs relative to it. The snapshot has an entry for every UUID in
// groupIDs and no others.
//
// Test code can supply a reader as an ordinary function that returns a
// snapshot value, with no database.
type GroupSnapshotReader func(ctx context.Context, orgID uuid.UUID, groupIDs []uuid.UUID) (GroupSnapshot, error)

// DatabaseGroupSnapshotReader returns the reader that reads from db. When
// the caller writes settings based on the snapshot, db should be the
// transaction that does the write, so that the snapshot and the write see
// the same state.
//
// Precondition: the ctx passed to the reader carries a system subject. The
// statuses are correct only when the queries see every organization and
// every group, not only those visible to a narrower subject; a group hidden
// from the subject would be reported as not existing.
func DatabaseGroupSnapshotReader(db database.Store) GroupSnapshotReader {
	return func(ctx context.Context, orgID uuid.UUID, groupIDs []uuid.UUID) (GroupSnapshot, error) {
		snapshot := GroupSnapshot{
			OrganizationID: orgID,
			Groups:         make(map[uuid.UUID]GroupStatus, len(groupIDs)),
		}

		org, err := db.GetOrganizationByID(ctx, orgID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			snapshot.Organization = OrganizationAbsent
		case err != nil:
			return GroupSnapshot{}, xerrors.Errorf("get organization: %w", err)
		case org.Deleted:
			snapshot.Organization = OrganizationDeleted
		default:
			snapshot.Organization = OrganizationActive
		}

		if len(groupIDs) == 0 {
			// GetGroups applies no ID filter to an empty list, so it is
			// queried only for a nonempty one.
			return snapshot, nil
		}
		// Assert: groupIDs is nonempty, so GetGroups filters by it. No
		// organization filter is given, so groups in every organization are
		// returned.
		rows, err := db.GetGroups(ctx, database.GetGroupsParams{GroupIds: groupIDs})
		if err != nil {
			return GroupSnapshot{}, xerrors.Errorf("get groups: %w", err)
		}
		owner := make(map[uuid.UUID]uuid.UUID, len(rows))
		for _, row := range rows {
			owner[row.Group.ID] = row.Group.OrganizationID
		}

		for _, id := range groupIDs {
			switch ownerID, ok := owner[id]; {
			case !ok:
				snapshot.Groups[id] = GroupDoesNotExist
			case ownerID == orgID:
				snapshot.Groups[id] = GroupInsideOrganization
			default:
				snapshot.Groups[id] = GroupOutsideOrganization
			}
		}
		return snapshot, nil
	}
}
