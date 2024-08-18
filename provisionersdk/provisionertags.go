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
// Otherwise, the "owner" tag is always an empty string.
// NOTE: "owner" must NEVER be nil. Otherwise it will end up being
// duplicated in the database, as idx_provisioner_daemons_name_owner_key
// is a partial unique index that includes a JSON field.
func MutateTags(userID uuid.UUID, provided map[string]string) map[string]string {
	tags := map[string]string{}
	for k, v := range provided {
		tags[k] = v
	}
	_, ok := tags[TagScope]
	if !ok {
		tags[TagScope] = ScopeOrganization
		tags[TagOwner] = ""
	}
	switch tags[TagScope] {
	case ScopeUser:
		tags[TagOwner] = userID.String()
	case ScopeOrganization:
		tags[TagOwner] = ""
	default:
		tags[TagScope] = ScopeOrganization
		tags[TagOwner] = ""
	}
	return tags
}
