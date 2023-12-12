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
func MutateTags(userID uuid.UUID, tags map[string]string) map[string]string {
	if tags == nil {
		tags = map[string]string{}
	}
	_, ok := tags[TagScope]
	if !ok {
		tags[TagScope] = ScopeOrganization
	}
	switch tags[TagScope] {
	case ScopeUser:
		tags[TagOwner] = userID.String()
	case ScopeOrganization:
	default:
		tags[TagScope] = ScopeOrganization
	}
	return tags
}
