package idpsync

import (
	"maps"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// OrganizationStatus is the state of an organization in a GroupSnapshot.
type OrganizationStatus string

const (
	// OrganizationActive: the organization exists and is not deleted.
	OrganizationActive OrganizationStatus = "active"
	// OrganizationDeleted: the organization exists and is soft-deleted.
	OrganizationDeleted OrganizationStatus = "deleted"
	// OrganizationAbsent: no organization has the ID.
	OrganizationAbsent OrganizationStatus = "absent"
)

// GroupStatus is the state of a group ID in a GroupSnapshot, relative to the
// organization the snapshot was read for.
type GroupStatus string

const (
	// GroupInsideOrganization: the ID names a group that belongs to the
	// snapshot's organization.
	GroupInsideOrganization GroupStatus = "inside_organization"
	// GroupOutsideOrganization: the ID names a group that belongs to another
	// organization.
	GroupOutsideOrganization GroupStatus = "outside_organization"
	// GroupDoesNotExist: the ID names no group.
	GroupDoesNotExist GroupStatus = "does_not_exist"
)

// GroupSnapshot is the state of the deployment that the full validator needs
// for one organization, read at one moment.
type GroupSnapshot struct {
	// OrganizationID is the organization the snapshot was read for, and the
	// ID of that organization's Everyone group.
	OrganizationID uuid.UUID
	// Organization is the state of that organization.
	Organization OrganizationStatus
	// Groups has an entry for every group ID the snapshot was read for,
	// giving that ID's status. An ID with no entry was not read.
	Groups map[uuid.UUID]GroupStatus
}

// ValidityFindings reports every violation of the validity conditions V1
// through V5 in settings, against snapshot. V1 is well-formedness, so the
// result includes settings.WellFormedFindings().
//
// The snapshot must have been read for at least the UUIDs that
// settings.CanonicalMappingUUIDs() returns. If any of them has no entry in
// snapshot.Groups, ValidityFindings returns an error and no findings, rather
// than guess the status of a UUID it was not given.
//
// Each array element of `mapping` has at most one finding: an element the
// syntax validator reports as the nil UUID or as a repeat gets no V3, V4,
// or V5 finding. A finding about the organization has the empty path. The
// findings are ordered as WellFormedFindings orders them.
func ValidityFindings(settings codersdk.GroupSyncSettings, snapshot GroupSnapshot) ([]codersdk.GroupSyncFinding, error) {
	// The snapshot must cover every ID before anything is reported, so that
	// a caller never acts on a partial result.
	for _, id := range settings.CanonicalMappingUUIDs() {
		if _, ok := snapshot.Groups[id]; !ok {
			return nil, xerrors.Errorf("snapshot has no entry for group %s", id)
		}
	}
	// Assert: every non-nil UUID in `mapping` has an entry in snapshot.Groups.

	found := settings.WellFormedFindings()

	// V2: the organization must exist and not be deleted.
	switch snapshot.Organization {
	case OrganizationActive:
	case OrganizationDeleted:
		found = append(found, codersdk.GroupSyncFinding{Code: codersdk.GroupSyncFindingOrganizationDeleted, Path: ""})
	case OrganizationAbsent:
		found = append(found, codersdk.GroupSyncFinding{Code: codersdk.GroupSyncFindingOrganizationAbsent, Path: ""})
	default:
		return nil, xerrors.Errorf("snapshot has unknown organization status %q", snapshot.Organization)
	}

	// V3, V4, and V5, element by element. An element the syntax validator
	// reports, the nil UUID or a repeat within its array, gets no finding
	// here, so each element has at most one.
	for _, key := range slices.Sorted(maps.Keys(settings.Mapping)) {
		seen := make(map[uuid.UUID]bool)
		for i, id := range settings.Mapping[key] {
			if id == uuid.Nil || seen[id] {
				continue
			}
			seen[id] = true

			var code codersdk.GroupSyncFindingCode
			switch status := snapshot.Groups[id]; status {
			case GroupDoesNotExist:
				code = codersdk.GroupSyncFindingMissingGroup
			case GroupOutsideOrganization:
				code = codersdk.GroupSyncFindingOutsideGroup
			case GroupInsideOrganization:
				// The Everyone group's ID is its organization's ID, and an
				// ID inside the organization that equals it is that group.
				if id == snapshot.OrganizationID {
					code = codersdk.GroupSyncFindingEveryoneGroup
				}
			default:
				return nil, xerrors.Errorf("snapshot has unknown status %q for group %s", status, id)
			}
			if code != "" {
				found = append(found, codersdk.GroupSyncFinding{Code: code, Path: mappingElementPath(key, i)})
			}
		}
	}

	slices.SortFunc(found, codersdk.CompareGroupSyncFindings)
	return found, nil
}

// mappingElementPath renders the JSON Pointer to element index of the
// `mapping` array under key.
func mappingElementPath(key string, index int) string {
	return "/mapping/" + codersdk.EscapeJSONPointerToken(key) + "/" + strconv.Itoa(index)
}
