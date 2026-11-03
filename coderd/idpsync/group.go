package idpsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

type GroupParams struct {
	// SyncEntitled if false will skip syncing the user's groups
	SyncEntitled bool
	MergedClaims jwt.MapClaims
}

func (AGPLIDPSync) GroupSyncEntitled() bool {
	// AGPL does not support syncing groups.
	return false
}

func (s AGPLIDPSync) UpdateGroupSyncSettings(ctx context.Context, orgID uuid.UUID, db database.Store, settings GroupSyncSettings) error {
	orgResolver := s.Manager.OrganizationResolver(db, orgID)
	err := s.Group.SetRuntimeValue(ctx, orgResolver, &settings)
	if err != nil {
		return xerrors.Errorf("update group sync settings: %w", err)
	}

	return nil
}

func (s AGPLIDPSync) GroupSyncSettings(ctx context.Context, orgID uuid.UUID, db database.Store) (*GroupSyncSettings, error) {
	orgResolver := s.Manager.OrganizationResolver(db, orgID)
	settings, err := s.Group.Resolve(ctx, orgResolver)
	if err != nil {
		if !xerrors.Is(err, runtimeconfig.ErrEntryNotFound) {
			return nil, xerrors.Errorf("resolve group sync settings: %w", err)
		}

		// Default to not being configured
		settings = &GroupSyncSettings{}

		// Check for legacy settings if the default org.
		if s.Legacy.GroupField != "" {
			defaultOrganization, err := db.GetDefaultOrganization(ctx)
			if err != nil {
				return nil, xerrors.Errorf("get default organization: %w", err)
			}
			if defaultOrganization.ID == orgID {
				settings = ptr.Ref(GroupSyncSettings(codersdk.GroupSyncSettings{
					Field:             s.Legacy.GroupField,
					LegacyNameMapping: s.Legacy.GroupMapping,
					RegexFilter:       s.Legacy.GroupFilter,
					AutoCreateMissing: s.Legacy.CreateMissingGroups,
				}))
			}
		}
	}

	return settings, nil
}

func (s AGPLIDPSync) ParseGroupClaims(_ context.Context, mergedClaims jwt.MapClaims) (GroupParams, *HTTPError) {
	if s.GroupField != "" && len(s.GroupAllowList) > 0 {
		groupsRaw, ok := mergedClaims[s.GroupField]
		if !ok {
			return GroupParams{}, &HTTPError{
				Code:             http.StatusForbidden,
				Msg:              "Not a member of an allowed group",
				Detail:           "You have no groups in your claims!",
				RenderStaticPage: true,
			}
		}
		parsedGroups, err := ParseStringSliceClaim(groupsRaw)
		if err != nil {
			return GroupParams{}, &HTTPError{
				Code:             http.StatusBadRequest,
				Msg:              "Failed read groups from claims for allow list check. Ask an administrator for help.",
				Detail:           err.Error(),
				RenderStaticPage: true,
			}
		}

		inAllowList := false
	AllowListCheckLoop:
		for _, group := range parsedGroups {
			if _, ok := s.GroupAllowList[group]; ok {
				inAllowList = true
				break AllowListCheckLoop
			}
		}

		if !inAllowList {
			return GroupParams{}, &HTTPError{
				Code:             http.StatusForbidden,
				Msg:              "Not a member of an allowed group",
				Detail:           "Ask an administrator to add one of your groups to the allow list.",
				RenderStaticPage: true,
			}
		}
	}

	return GroupParams{
		SyncEntitled: s.GroupSyncEntitled(),
		MergedClaims: mergedClaims,
	}, nil
}

func (s AGPLIDPSync) SyncGroups(ctx context.Context, db database.Store, user database.User, params GroupParams) error {
	// Nothing happens if sync is not enabled
	if !params.SyncEntitled {
		return nil
	}

	// nolint:gocritic // all syncing is done as a system user
	ctx = dbauthz.AsSystemRestricted(ctx)

	err := db.InTx(func(tx database.Store) error {
		userGroups, err := tx.GetGroups(ctx, database.GetGroupsParams{
			HasMemberID: user.ID,
		})
		if err != nil {
			return xerrors.Errorf("get user groups: %w", err)
		}

		// Figure out which organizations the user is a member of.
		// The "Everyone" group is always included, so we can infer organization
		// membership via the groups the user is in.
		userOrgs := make(map[uuid.UUID][]database.GetGroupsRow)
		for _, g := range userGroups {
			userOrgs[g.Group.OrganizationID] = append(userOrgs[g.Group.OrganizationID], g)
		}

		// For each org, we need to fetch the sync settings
		// This loop also handles any legacy settings for the default
		// organization.
		orgSettings := make(map[uuid.UUID]GroupSyncSettings)
		for orgID := range userOrgs {
			settings, err := s.GroupSyncSettings(ctx, orgID, tx)
			if err != nil {
				// TODO: This error is currently silent to org admins.
				// We need to come up with a way to notify the org admin of this
				// error.
				s.Logger.Error(ctx, "failed to get group sync settings",
					slog.F("organization_id", orgID),
					slog.Error(err),
				)
				settings = &GroupSyncSettings{}
			}
			orgSettings[orgID] = *settings
		}

		// groupIDsToAdd & groupIDsToRemove are the final group differences
		// needed to be applied to user. The loop below will iterate over all
		// organizations the user is in, and determine the diffs.
		// The diffs are applied as a batch sql query, rather than each
		// organization having to execute a query.
		groupIDsToAdd := make([]uuid.UUID, 0)
		groupIDsToRemove := make([]uuid.UUID, 0)
		// For each org, determine which groups the user should land in
		for orgID, settings := range orgSettings {
			if settings.Field == "" {
				// No group sync enabled for this org, so do nothing.
				// The user can remain in their groups for this org.
				continue
			}

			// expectedGroups is the set of groups the IDP expects the
			// user to be a member of.
			expectedGroups, err := settings.ParseClaims(orgID, params.MergedClaims)
			if err != nil {
				s.Logger.Debug(ctx, "failed to parse claims for groups",
					slog.F("organization_field", s.GroupField),
					slog.F("organization_id", orgID),
					slog.Error(err),
				)
				// Unsure where to raise this error on the UI or database.
				// TODO: This error prevents group sync, but we have no way
				// to raise this to an org admin. Come up with a solution to
				// notify the admin and user of this issue.
				continue
			}

			// Settings can name groups that are in another organization or
			// that do not exist. Only groups in this organization are synced;
			// the rest are reported and dropped.
			expectedGroups, outside, missing, err := partitionOutInvalidSettings(ctx, tx, orgID, expectedGroups)
			if err != nil {
				return xerrors.Errorf("partition expected groups: %w", err)
			}
			s.logInvalidGroupSetting(ctx, "outside", orgID, user.ID, outside)
			s.logInvalidGroupSetting(ctx, "missing", orgID, user.ID, missing)

			// Everyone group is always implied, so include it.
			expectedGroups = append(expectedGroups, ExpectedGroup{
				OrganizationID: orgID,
				GroupID:        &orgID,
			})

			// Now we know what groups the user should be in for a given org,
			// determine if we have to do any group updates to sync the user's
			// state.
			existingGroups := userOrgs[orgID]
			existingGroupsTyped := slice.List(existingGroups, func(f database.GetGroupsRow) ExpectedGroup {
				return ExpectedGroup{
					OrganizationID: orgID,
					GroupID:        &f.Group.ID,
					GroupName:      &f.Group.Name,
				}
			})

			add, remove := slice.SymmetricDifferenceFunc(existingGroupsTyped, expectedGroups, func(a, b ExpectedGroup) bool {
				return a.Equal(b)
			})

			for _, r := range remove {
				if r.GroupID == nil {
					// This should never happen. All group removals come from the
					// existing set, which come from the db. All groups from the
					// database have IDs. This code is purely defensive.
					detail := "user:" + user.Username
					if r.GroupName != nil {
						detail += fmt.Sprintf(" from group %s", *r.GroupName)
					}
					return xerrors.Errorf("removal group has nil ID, which should never happen: %s", detail)
				}
				groupIDsToRemove = append(groupIDsToRemove, *r.GroupID)
			}

			// HandleMissingGroups will add the new groups to the org if
			// the settings specify. It will convert all group names into uuids
			// for easier assignment.
			// TODO: This code should be batched at the end of the for loop.
			// Optimizing this is being pushed because if AutoCreate is disabled,
			// this code will only add cost on the first login for each user.
			// AutoCreate is usually disabled for large deployments.
			// For small deployments, this is less of a problem.
			assignGroups, err := settings.HandleMissingGroups(ctx, tx, orgID, add)
			if err != nil {
				return xerrors.Errorf("handle missing groups: %w", err)
			}

			groupIDsToAdd = append(groupIDsToAdd, assignGroups...)
		}

		// ApplyGroupDifference will take the total adds and removes, and apply
		// them.
		err = s.ApplyGroupDifference(ctx, tx, user, groupIDsToAdd, groupIDsToRemove)
		if err != nil {
			return xerrors.Errorf("apply group difference: %w", err)
		}

		return nil
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

// logInvalidGroupSetting warns that an organization's group sync settings
// name groups that were not synced. kind is "outside" for groups in another
// organization and "missing" for groups that do not exist. Nothing is logged
// when groups is empty.
func (s AGPLIDPSync) logInvalidGroupSetting(ctx context.Context, kind string, orgID, userID uuid.UUID, groups []ExpectedGroup) {
	if len(groups) == 0 {
		return
	}
	s.Logger.Warn(ctx, "group sync settings name groups that were not synced",
		slog.F("invalid_group_setting", kind),
		slog.F("organization_id", orgID),
		slog.F("user_id", userID),
		slog.F("group_ids", slice.List(groups, func(g ExpectedGroup) uuid.UUID {
			return *g.GroupID
		})),
	)
}

// ApplyGroupDifference will add and remove the user from the specified groups.
func (s AGPLIDPSync) ApplyGroupDifference(ctx context.Context, tx database.Store, user database.User, add []uuid.UUID, removeIDs []uuid.UUID) error {
	if len(removeIDs) > 0 {
		removedGroupIDs, err := tx.RemoveUserFromGroups(ctx, database.RemoveUserFromGroupsParams{
			UserID:   user.ID,
			GroupIds: removeIDs,
		})
		if err != nil {
			return xerrors.Errorf("remove user from %d groups: %w", len(removeIDs), err)
		}
		if len(removedGroupIDs) != len(removeIDs) {
			s.Logger.Debug(ctx, "user not removed from expected number of groups",
				slog.F("user_id", user.ID),
				slog.F("groups_removed_count", len(removedGroupIDs)),
				slog.F("expected_count", len(removeIDs)),
			)
		}
	}

	if len(add) > 0 {
		add = slice.Unique(add)
		// Defensive programming to only insert uniques.
		assignedGroupIDs, err := tx.InsertUserGroupsByID(ctx, database.InsertUserGroupsByIDParams{
			UserID:   user.ID,
			GroupIds: add,
		})
		if err != nil {
			return xerrors.Errorf("insert user into %d groups: %w", len(add), err)
		}
		if len(assignedGroupIDs) != len(add) {
			s.Logger.Debug(ctx, "user not assigned to expected number of groups",
				slog.F("user_id", user.ID),
				slog.F("groups_assigned_count", len(assignedGroupIDs)),
				slog.F("expected_count", len(add)),
			)
		}
	}

	return nil
}

type GroupSyncSettings codersdk.GroupSyncSettings

func (s *GroupSyncSettings) Set(v string) error {
	return json.Unmarshal([]byte(v), s)
}

func (s *GroupSyncSettings) String() string {
	if s.Mapping == nil {
		s.Mapping = make(map[string][]uuid.UUID)
	}
	return runtimeconfig.JSONString(s)
}

func (s *GroupSyncSettings) MarshalJSON() ([]byte, error) {
	if s.Mapping == nil {
		s.Mapping = make(map[string][]uuid.UUID)
	}

	// Aliasing the struct to avoid infinite recursion when calling json.Marshal
	// on the struct itself.
	type Alias GroupSyncSettings
	return json.Marshal(&struct{ *Alias }{Alias: (*Alias)(s)})
}

type ExpectedGroup struct {
	OrganizationID uuid.UUID
	GroupID        *uuid.UUID
	GroupName      *string
}

// Equal compares two ExpectedGroups. The org id must be the same.
// If the group ID is set, it will be compared and take priority, ignoring the
// name value. So 2 groups with the same ID but different names will be
// considered equal.
func (a ExpectedGroup) Equal(b ExpectedGroup) bool {
	// Must match
	if a.OrganizationID != b.OrganizationID {
		return false
	}
	// Only the name or the name needs to be checked, priority is given to the ID.
	if a.GroupID != nil && b.GroupID != nil {
		return *a.GroupID == *b.GroupID
	}
	if a.GroupName != nil && b.GroupName != nil {
		return *a.GroupName == *b.GroupName
	}

	// If everything is nil, it is equal. Although a bit pointless
	if a.GroupID == nil && b.GroupID == nil &&
		a.GroupName == nil && b.GroupName == nil {
		return true
	}
	return false
}

// ParseClaims will take the merged claims from the IDP and return the groups
// the user is expected to be a member of. The expected group can either be a
// name or an ID.
// It is unfortunate we cannot use exclusively names or exclusively IDs.
// When configuring though, if a group is mapped from "A" -> "UUID 1234", and
// the group "UUID 1234" is renamed, we want to maintain the mapping.
// We have to keep names because group sync supports syncing groups by name if
// the external IDP group name matches the Coder one.
func (s GroupSyncSettings) ParseClaims(orgID uuid.UUID, mergedClaims jwt.MapClaims) ([]ExpectedGroup, error) {
	groupsRaw, ok := mergedClaims[s.Field]
	if !ok {
		return []ExpectedGroup{}, nil
	}

	parsedGroups, err := ParseStringSliceClaim(groupsRaw)
	if err != nil {
		return nil, xerrors.Errorf("parse groups field, unexpected type %T: %w", groupsRaw, err)
	}

	groups := make([]ExpectedGroup, 0)
	for _, group := range parsedGroups {
		// Legacy group mappings happen before the regex filter.
		mappedGroupName, ok := s.LegacyNameMapping[group]
		if ok {
			group = mappedGroupName
		}

		// Only allow through groups that pass the regex
		if s.RegexFilter != nil {
			if !s.RegexFilter.MatchString(group) {
				continue
			}
		}

		mappedGroupIDs, ok := s.Mapping[group]
		if ok {
			for _, gid := range mappedGroupIDs {
				groups = append(groups, ExpectedGroup{OrganizationID: orgID, GroupID: &gid})
			}
			continue
		}

		groups = append(groups, ExpectedGroup{OrganizationID: orgID, GroupName: &group})
	}

	return groups, nil
}

// HandleMissingGroups ensures all ExpectedGroups convert to uuids.
// Groups can be referenced by name via legacy params or IDP group names.
// These group names are converted to IDs for easier assignment.
// Missing groups are created if AutoCreate is enabled.
// TODO: Batching this would be better, as this is 1 or 2 db calls per organization.
func (s GroupSyncSettings) HandleMissingGroups(ctx context.Context, tx database.Store, orgID uuid.UUID, add []ExpectedGroup) ([]uuid.UUID, error) {
	// All expected that are missing IDs means the group does not exist
	// in the database, or it is a legacy mapping, and we need to do a lookup.
	var missingGroups []string
	addIDs := make([]uuid.UUID, 0)

	for _, expected := range add {
		if expected.GroupID == nil && expected.GroupName != nil {
			missingGroups = append(missingGroups, *expected.GroupName)
		} else if expected.GroupID != nil {
			// Keep the IDs to sync the groups.
			addIDs = append(addIDs, *expected.GroupID)
		}
	}

	if s.AutoCreateMissing && len(missingGroups) > 0 {
		// Insert any missing groups. If the groups already exist, this is a noop.
		_, err := tx.InsertMissingGroups(ctx, database.InsertMissingGroupsParams{
			OrganizationID: orgID,
			Source:         database.GroupSourceOidc,
			GroupNames:     missingGroups,
		})
		if err != nil {
			return nil, xerrors.Errorf("insert missing groups: %w", err)
		}
	}

	// Fetch any missing groups by name. If they exist, their IDs will be
	// matched and returned.
	if len(missingGroups) > 0 {
		// Do name lookups for all groups that are missing IDs.
		newGroups, err := tx.GetGroups(ctx, database.GetGroupsParams{
			OrganizationID: orgID,
			HasMemberID:    uuid.UUID{},
			GroupNames:     missingGroups,
		})
		if err != nil {
			return nil, xerrors.Errorf("get groups by names: %w", err)
		}
		for _, g := range newGroups {
			addIDs = append(addIDs, g.Group.ID)
		}
	}

	return addIDs, nil
}

// partitionOutInvalidSettings splits the expected groups for an organization
// into three disjoint lists. Inside holds entries whose GroupID is a group in
// orgID, plus entries that carry only a name, since names are resolved within
// the organization by HandleMissingGroups. Outside holds entries whose GroupID
// is a group in some other organization. Missing holds entries whose GroupID
// matches no group at all, whether deleted or never created. Only Inside is
// safe to sync; Outside and Missing come from invalid sync settings.
//
// Precondition: This function is called within a system context.
// Correctness of the partition requires that the queries for `GetGroups` be
// made over all groups, not only over a subset visible to a narrower subject.
func partitionOutInvalidSettings(ctx context.Context, tx database.Store, orgID uuid.UUID, expected []ExpectedGroup) (inside, outside, missing []ExpectedGroup, err error) {
	var ids []uuid.UUID
	for _, e := range expected {
		if e.GroupID != nil {
			ids = append(ids, *e.GroupID)
		}
	}
	if len(ids) == 0 {
		// `GetGroups` applies no ID filter when `GroupIds` is empty, so each
		// lookup runs only on a nonempty list of IDs. With no IDs, every
		// entry is a name, and names are inside.
		return expected, nil, nil, nil
	}
	// Assert: `ids` is nonempty.

	inOrg := make(map[uuid.UUID]struct{})
	// Because `ids` is nonempty, `GetGroups` will filter its results.
	rows, err := tx.GetGroups(ctx, database.GetGroupsParams{
		OrganizationID: orgID,
		GroupIds:       ids,
	})
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("get groups in organization: %w", err)
	}
	for _, row := range rows {
		inOrg[row.Group.ID] = struct{}{}
	}
	var rest []uuid.UUID
	for _, id := range ids {
		if _, ok := inOrg[id]; !ok {
			rest = append(rest, id)
		}
	}
	if len(rest) == 0 {
		// Every ID is in the organization, which is the ordinary case.
		return expected, nil, nil, nil
	}
	// Assert: `rest` is nonempty.
	// Assert: `expected` contains at least one group ID that doesn't belong in
	//   `inside`.

	elsewhere := make(map[uuid.UUID]struct{})
	// This query distinguishes between `outside` and `missing` group IDs. This
	// is to benefit the administrator, to better document how the sync
	// settings are invalid.
	rows, err = tx.GetGroups(ctx, database.GetGroupsParams{
		GroupIds: rest,
	})
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("get groups outside organization: %w", err)
	}
	for _, row := range rows {
		elsewhere[row.Group.ID] = struct{}{}
	}

	for _, e := range expected {
		if e.GroupID == nil {
			inside = append(inside, e)
			continue
		}
		if _, ok := inOrg[*e.GroupID]; ok {
			inside = append(inside, e)
		} else if _, ok := elsewhere[*e.GroupID]; ok {
			outside = append(outside, e)
		} else {
			missing = append(missing, e)
		}
	}
	return inside, outside, missing, nil
}

func ConvertAllowList(allowList []string) map[string]struct{} {
	allowMap := make(map[string]struct{}, len(allowList))
	for _, group := range allowList {
		allowMap[group] = struct{}{}
	}
	return allowMap
}
