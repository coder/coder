package database

import "github.com/coder/coder/v2/coderd/rbac"

// RBACObject retains artifact authorization independently of its workspace.
func (a ReadWorkspaceExecutionArtifactRow) RBACObject() rbac.Object {
	return rbac.ResourceWorkspaceExecution.WithID(a.SessionID).WithOwner(a.OwnerID.String()).InOrg(a.OrganizationID)
}

// RBACObject retains artifact metadata authorization after workspace deletion.
func (a GetWorkspaceExecutionArtifactsBySessionIDRow) RBACObject() rbac.Object {
	return rbac.ResourceWorkspaceExecution.WithID(a.SessionID).WithOwner(a.OwnerID.String()).InOrg(a.OrganizationID)
}
