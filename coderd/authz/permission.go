package authz

type Permission struct {
	// Negate makes this a negative permission
	Negate       bool
	ResourceType ResourceType
	ResourceID   string
	Action       Action
}
