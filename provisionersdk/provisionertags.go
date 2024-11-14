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
// Multiple sets of tags may be passed to this function; they will
// be merged into one single tag set.
// Otherwise, the "owner" tag is always an empty string.
// NOTE: "owner" must NEVER be nil. Otherwise it will end up being
// duplicated in the database, as idx_provisioner_daemons_name_owner_key
// is a partial unique index that includes a JSON field.
func MutateTags(userID uuid.UUID, provided ...map[string]string) map[string]string {
	tags := map[string]string{}
	for _, m := range provided {
		tags = mergeTags(tags, m)
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

// mergeTags merges multiple sets of provisioner tags.
// Given maps a and b, the result will contain all
// keys and values contained in a and in b.
// An exception is made for key-value pairs for which
// a[key] is non-empty but b[key] is empty. In this case
// the non-empty value from a will win.
func mergeTags(a, b map[string]string) map[string]string {
	m := make(map[string]string)
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		if v == "" {
			continue
		}
		m[k] = v
	}
	return m
}
