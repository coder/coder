package rbac

// Permission is the format passed into the rego.
type Permission struct {
	// Negate makes this a negative permission
	Negate       bool   `json:"negate"`
	ResourceType string `json:"resource_type"`
	Action       Action `json:"action"`
}

// Role is a set of permissions at multiple levels:
// - Site level permissions apply EVERYWHERE
// - Org level permissions apply to EVERYTHING in a given ORG
// - User level permissions are the lowest
// This is the type passed into the rego as a json payload.
// Users of this package should instead **only** use the role names, and
// this package will expand the role names into their json payloads.
type Role struct {
	Name string `json:"name"`
	// DisplayName is used for UI purposes. If the role has no display name,
	// that means the UI should never display it.
	DisplayName string       `json:"display_name"`
	Site        []Permission `json:"site"`
	// Org is a map of orgid to permissions. We represent orgid as a string.
	// We scope the organizations in the role so we can easily combine all the
	// roles.
	Org  map[string][]Permission `json:"org"`
	User []Permission            `json:"user"`
}
