package provisionersdk

import "github.com/google/uuid"

const (
	TagScope = "scope"
	TagOwner = "owner"

	ScopeUser         = "user"
	ScopeOrganization = "organization"
)

// MutateTags adjusts the "owner" tag dependent on the "scope".
// If the scope is "user", the "owner" is changed to the user ID.
// This is for user-scoped provisioner daemons, where users should
// own their own operations.
// Otherwise, the "owner" tag is always empty.
func MutateTags(userID uuid.UUID, tags map[string]string) map[string]string {
	// We copy the tags here to avoid overwriting the provided map. This can
	// cause data races when using dbmem.
	cp := map[string]string{}
	for k, v := range tags {
		cp[k] = v
	}

	_, ok := cp[TagScope]
	if !ok {
		cp[TagScope] = ScopeOrganization
		delete(cp, TagOwner)
	}
	switch cp[TagScope] {
	case ScopeUser:
		cp[TagOwner] = userID.String()
	case ScopeOrganization:
		delete(cp, TagOwner)
	default:
		cp[TagScope] = ScopeOrganization
	}
	return cp
}
