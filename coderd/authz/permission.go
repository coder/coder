package authz

type Permission struct {
	// Negate makes this a negative permission
	Negate bool
	// OrganizationID is used for identifying a particular org.
	//	org:1234
	OrganizationID string

	ResourceType ResourceType
	ResourceID   string
	Action       Action
}
