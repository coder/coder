package idp_sync

import (
	"context"
	"regexp"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

type IDPSync struct {
	*codersdk.Entitlement
}

// SynchronizeGroupsParams
type SynchronizeGroupsParams struct {
	IDPGroups []string
	// TODO: These options will be moved outside of deployment into organization
	// scoped settings. So these parameters will be removed, and instead sourced
	// from some settings object that should have these values cached.
	// At present, these settings apply to the default organization.
	GroupFilter         *regexp.Regexp
	CreateMissingGroups bool
}

// SynchronizeGroups takes a given user, and ensures their group memberships
// within a given organization are correct.
func SynchronizeGroups(ctx context.Context, tx database.Store, params *SynchronizeGroupsParams) error {

	return nil
}
