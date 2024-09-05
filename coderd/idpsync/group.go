package idpsync

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/util/slice"
)

type GroupParams struct {
	// SyncEnabled if false will skip syncing the user's groups
	SyncEnabled  bool
	MergedClaims jwt.MapClaims
}

func (AGPLIDPSync) GroupSyncEnabled() bool {
	// AGPL does not support syncing groups.
	return false
}

func (s AGPLIDPSync) ParseGroupClaims(_ context.Context, _ jwt.MapClaims) (GroupParams, *HTTPError) {
	return GroupParams{
		SyncEnabled: s.GroupSyncEnabled(),
	}, nil
}

func (s AGPLIDPSync) SyncGroups(ctx context.Context, db database.Store, user database.User, params GroupParams) error {
	// Nothing happens if sync is not enabled
	if !params.SyncEnabled {
		return nil
	}

	// Only care about the default org for deployment settings if the
	// legacy deployment settings exist.
	defaultOrgID := uuid.Nil
	// Default organization is configured via legacy deployment values
	if s.DeploymentSyncSettings.Legacy.GroupField != "" {
		defaultOrganization, err := db.GetDefaultOrganization(ctx)
		if err != nil {
			return xerrors.Errorf("get default organization: %w", err)
		}
		defaultOrgID = defaultOrganization.ID
	}

	// nolint:gocritic // all syncing is done as a system user
	ctx = dbauthz.AsSystemRestricted(ctx)

	db.InTx(func(tx database.Store) error {
		userGroups, err := tx.GetGroups(ctx, database.GetGroupsParams{
			HasMemberID: user.ID,
		})
		if err != nil {
			return xerrors.Errorf("get user groups: %w", err)
		}

		// Figure out which organizations the user is a member of.
		userOrgs := make(map[uuid.UUID][]database.GetGroupsRow)
		for _, g := range userGroups {
			g := g
			userOrgs[g.Group.OrganizationID] = append(userOrgs[g.Group.OrganizationID], g)
		}

		// For each org, we need to fetch the sync settings
		orgSettings := make(map[uuid.UUID]GroupSyncSettings)
		for orgID := range userOrgs {
			orgResolver := s.Manager.OrganizationResolver(tx, orgID)
			settings, err := s.SyncSettings.Group.Resolve(ctx, orgResolver)
			if err != nil {
				return xerrors.Errorf("resolve group sync settings: %w", err)
			}
			orgSettings[orgID] = *settings

			// Legacy deployment settings will override empty settings.
			if orgID == defaultOrgID && settings.GroupField == "" {
				settings = &GroupSyncSettings{
					GroupField:              s.Legacy.GroupField,
					LegacyGroupNameMapping:  s.Legacy.GroupMapping,
					RegexFilter:             s.Legacy.GroupFilter,
					AutoCreateMissingGroups: s.Legacy.CreateMissingGroups,
				}
			}
		}

		// collect all diffs to do 1 sql update for all orgs
		groupsToAdd := make([]uuid.UUID, 0)
		groupsToRemove := make([]uuid.UUID, 0)
		// For each org, determine which groups the user should land in
		for orgID, settings := range orgSettings {
			if settings.GroupField == "" {
				// No group sync enabled for this org, so do nothing.
				continue
			}

			expectedGroups, err := settings.ParseClaims(params.MergedClaims)
			if err != nil {
				s.Logger.Debug(ctx, "failed to parse claims for groups",
					slog.F("organization_field", s.GroupField),
					slog.F("organization_id", orgID),
					slog.Error(err),
				)
				// Unsure where to raise this error on the UI or database.
				continue
			}
			// Everyone group is always implied.
			expectedGroups = append(expectedGroups, ExpectedGroup{
				GroupID: &orgID,
			})

			// Now we know what groups the user should be in for a given org,
			// determine if we have to do any group updates to sync the user's
			// state.
			existingGroups := userOrgs[orgID]
			existingGroupsTyped := db2sdk.List(existingGroups, func(f database.GetGroupsRow) ExpectedGroup {
				return ExpectedGroup{
					GroupID:   &f.Group.ID,
					GroupName: &f.Group.Name,
				}
			})
			add, remove := slice.SymmetricDifferenceFunc(existingGroupsTyped, expectedGroups, func(a, b ExpectedGroup) bool {
				// Only the name or the name needs to be checked, priority is given to the ID.
				if a.GroupID != nil && b.GroupID != nil {
					return *a.GroupID == *b.GroupID
				}
				if a.GroupName != nil && b.GroupName != nil {
					return *a.GroupName == *b.GroupName
				}
				return false
			})

			// HandleMissingGroups will add the new groups to the org if
			// the settings specify. It will convert all group names into uuids
			// for easier assignment.
			assignGroups, err := settings.HandleMissingGroups(ctx, tx, orgID, add)
			if err != nil {
				return xerrors.Errorf("handle missing groups: %w", err)
			}

			for _, removeGroup := range remove {
				// This should always be the case.
				// TODO: make sure this is always the case
				if removeGroup.GroupID != nil {
					groupsToRemove = append(groupsToRemove, *removeGroup.GroupID)
				}
			}

			groupsToAdd = append(groupsToAdd, assignGroups...)
		}

		assignedGroupIDs, err := tx.InsertUserGroupsByID(ctx, database.InsertUserGroupsByIDParams{
			UserID:   user.ID,
			GroupIds: groupsToAdd,
		})
		if err != nil {
			return xerrors.Errorf("insert user into %d groups: %w", len(groupsToAdd), err)
		}
		if len(assignedGroupIDs) != len(groupsToAdd) {
			s.Logger.Debug(ctx, "failed to assign all groups to user",
				slog.F("user_id", user.ID),
				slog.F("groups_assigned_count", len(assignedGroupIDs)),
				slog.F("expected_count", len(groupsToAdd)),
			)
		}

		removedGroupIDs, err := tx.RemoveUserFromGroups(ctx, database.RemoveUserFromGroupsParams{
			UserID:   user.ID,
			GroupIds: groupsToRemove,
		})
		if err != nil {
			return xerrors.Errorf("remove user from %d groups: %w", len(groupsToRemove), err)
		}
		if len(removedGroupIDs) != len(groupsToRemove) {
			s.Logger.Debug(ctx, "failed to remove user from all groups",
				slog.F("user_id", user.ID),
				slog.F("groups_removed_count", len(removedGroupIDs)),
				slog.F("expected_count", len(groupsToRemove)),
			)
		}

		return nil
	}, nil)

	return nil
}

type GroupSyncSettings struct {
	GroupField string `json:"field"`
	// GroupMapping maps from an OIDC group --> Coder group ID
	GroupMapping            map[string][]uuid.UUID `json:"mapping"`
	RegexFilter             *regexp.Regexp         `json:"regex_filter"`
	AutoCreateMissingGroups bool                   `json:"auto_create_missing_groups"`
	// LegacyGroupNameMapping is deprecated. It remaps an IDP group name to
	// a Coder group name. Since configuration is now done at runtime,
	// group IDs are used to account for group renames.
	// For legacy configurations, this config option has to remain.
	// Deprecated: Use GroupMapping instead.
	LegacyGroupNameMapping map[string]string
}

func (s *GroupSyncSettings) Set(v string) error {
	return json.Unmarshal([]byte(v), s)
}
func (s *GroupSyncSettings) String() string {
	v, err := json.Marshal(s)
	if err != nil {
		return "decode failed: " + err.Error()
	}
	return string(v)
}
func (s *GroupSyncSettings) Type() string {
	return "GroupSyncSettings"
}

type ExpectedGroup struct {
	GroupID   *uuid.UUID
	GroupName *string
}

// ParseClaims will take the merged claims from the IDP and return the groups
// the user is expected to be a member of. The expected group can either be a
// name or an ID.
// It is unfortunate we cannot use exclusively names or exclusively IDs.
// When configuring though, if a group is mapped from "A" -> "UUID 1234", and
// the group "UUID 1234" is renamed, we want to maintain the mapping.
// We have to keep names because group sync supports syncing groups by name if
// the external IDP group name matches the Coder one.
func (s GroupSyncSettings) ParseClaims(mergedClaims jwt.MapClaims) ([]ExpectedGroup, error) {
	groupsRaw, ok := mergedClaims[s.GroupField]
	if !ok {
		return []ExpectedGroup{}, nil
	}

	parsedGroups, err := ParseStringSliceClaim(groupsRaw)
	if err != nil {
		return nil, xerrors.Errorf("parse groups field, unexpected type %T: %w", groupsRaw, err)
	}

	groups := make([]ExpectedGroup, 0)
	for _, group := range parsedGroups {
		// Only allow through groups that pass the regex
		if s.RegexFilter != nil {
			if !s.RegexFilter.MatchString(group) {
				continue
			}
		}

		mappedGroupIDs, ok := s.GroupMapping[group]
		if ok {
			for _, gid := range mappedGroupIDs {
				gid := gid
				groups = append(groups, ExpectedGroup{GroupID: &gid})
			}
			continue
		}

		mappedGroupName, ok := s.LegacyGroupNameMapping[group]
		if ok {
			groups = append(groups, ExpectedGroup{GroupName: &mappedGroupName})
			continue
		}
		group := group
		groups = append(groups, ExpectedGroup{GroupName: &group})
	}

	return groups, nil
}

func (s GroupSyncSettings) HandleMissingGroups(ctx context.Context, tx database.Store, orgID uuid.UUID, add []ExpectedGroup) ([]uuid.UUID, error) {
	if !s.AutoCreateMissingGroups {
		// construct the list of groups to search by name to see if they exist.
		var lookups []string
		filter := make([]uuid.UUID, 0)
		for _, expected := range add {
			if expected.GroupID != nil {
				filter = append(filter, *expected.GroupID)
			} else if expected.GroupName != nil {
				lookups = append(lookups, *expected.GroupName)
			}
		}

		if len(lookups) > 0 {
			newGroups, err := tx.GetGroups(ctx, database.GetGroupsParams{
				OrganizationID: uuid.UUID{},
				HasMemberID:    uuid.UUID{},
				GroupNames:     lookups,
			})
			if err != nil {
				return nil, xerrors.Errorf("get groups by names: %w", err)
			}
			for _, g := range newGroups {
				filter = append(filter, g.Group.ID)
			}
		}

		return filter, nil
	}

	// All expected that are missing IDs means the group does not exist
	// in the database. Either remove them, or create them if auto create is
	// turned on.
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

	createdMissingGroups, err := tx.InsertMissingGroups(ctx, database.InsertMissingGroupsParams{
		OrganizationID: orgID,
		Source:         database.GroupSourceOidc,
		GroupNames:     missingGroups,
	})
	if err != nil {
		return nil, xerrors.Errorf("insert missing groups: %w", err)
	}
	for _, created := range createdMissingGroups {
		addIDs = append(addIDs, created.ID)
	}

	return addIDs, nil
}

func ConvertAllowList(allowList []string) map[string]struct{} {
	allowMap := make(map[string]struct{}, len(allowList))
	for _, group := range allowList {
		allowMap[group] = struct{}{}
	}
	return allowMap
}
