package workspacequota

type Enforcer interface {
	UserWorkspaceLimit() int
	CanCreateWorkspace(count int) bool
}

type nop struct{}

func NewNop() Enforcer {
	return &nop{}
}

func (*nop) UserWorkspaceLimit() int {
	return 0
}
func (*nop) CanCreateWorkspace(_ int) bool {
	return true
}
