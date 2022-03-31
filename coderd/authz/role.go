package authz

type Role struct {
	Level       permLevel
	Permissions []Permission
}
