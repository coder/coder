package authz

//
//// Subject is the actor that is performing the action on an object
//type Subject struct {
//	UserID string `json:"user_id"`
//
//		SiteRoles []Role `json:"site_roles"`
//
//	// Ops are mapped for the resource and the list of operations on the resource for the scope.
//	SiteOps []Permission `json:"site_ops"`
//	OrgOps  []Permission `json:"org_ops"`
//	// UserOps only affect objects owned by the user
//	UserOps []Permission `json:"user_ops"`
//}
//
//func (s Subject) AllPermissions() []Permission{
//	// Explosion of roles + scopes
//	return []Permission{}
//}
//
//// Authn
//type S struct {
//	SiteRoles()  ([]rbac.Roles, error)
//	OrgRoles(ctx context.Context, orgID string) ([]rbac.Roles, error)
//	UserRoles()  ([]rbac.Roles, error)
//	Scopes()     ([]rbac.ResourcePermission, error)
//}
