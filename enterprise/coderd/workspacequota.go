package coderd

import "github.com/coder/coder/coderd/workspacequota"

type enforcer struct {
	userWorkspaceLimit int
}

func NewEnforcer(userWorkspaceLimit int) workspacequota.Enforcer {
	return &enforcer{
		userWorkspaceLimit: userWorkspaceLimit,
	}
}

func (e *enforcer) UserWorkspaceLimit() int {
	return e.userWorkspaceLimit
}

func (e *enforcer) CanCreateWorkspace(count int) bool {
	if e.userWorkspaceLimit == 0 {
		return true
	}

	return count < e.userWorkspaceLimit
}
