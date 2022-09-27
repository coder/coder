package workspacequota

type Enforcer interface {
	UserWorkspaceLimit() int
	CanCreateWorkspace(count int) bool
}

type nop struct{}

func NewNop() *nop {
	return &nop{}
}

func (_ *nop) UserWorkspaceLimit() int {
	return 0
}
func (_ *nop) CanCreateWorkspace(_ int) bool {
	return true
}
