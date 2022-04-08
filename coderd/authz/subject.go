package authz

// Subject is the actor that is attempting to do some action on some object or
// set of objects.
type Subject interface {
	// ID is the ID for the given actor. If it matches the OwnerID ID of the
	// object, we can assume the object is owned by this subject.
	ID() string

	GetRoles() ([]Role, error)
}

// SubjectTODO is a placeholder until we get an actual actor struct in place.
// This will come with the Authn epic.
// TODO: @emyrk delete this data structure when authn exists
type SubjectTODO struct {
	UserID string `json:"user_id"`

	Roles []Role
}

func (s SubjectTODO) ID() string {
	return s.UserID
}

func (s SubjectTODO) GetRoles() ([]Role, error) {
	return s.Roles, nil
}

func (SubjectTODO) Scopes() ([]Permission, error) {
	return []Permission{}, nil
}
