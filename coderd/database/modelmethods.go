package database

import "github.com/coder/coder/coderd/rbac"

func (t Template) RBACObject() rbac.Object {
	return rbac.ResourceTemplate.InOrg(t.OrganizationID).WithID(t.ID.String())
}

func (t TemplateVersion) RBACObject() rbac.Object {
	// Just use the parent template resource for controlling versions
	return rbac.ResourceTemplate.InOrg(t.OrganizationID).WithID(t.TemplateID.UUID.String())
}

func (w Workspace) RBACObject() rbac.Object {
	return rbac.ResourceWorkspace.InOrg(w.OrganizationID).WithID(w.ID.String()).WithOwner(w.OwnerID.String())
}

func (m OrganizationMember) RBACObject() rbac.Object {
	return rbac.ResourceOrganizationMember.InOrg(m.OrganizationID).WithID(m.UserID.String())
}

func (o Organization) RBACObject() rbac.Object {
	return rbac.ResourceOrganization.InOrg(o.ID).WithID(o.ID.String())
}

func (d ProvisionerDaemon) RBACObject() rbac.Object {
	return rbac.ResourceProvisionerDaemon.WithID(d.ID.String())
}

func (f File) RBACObject() rbac.Object {
	return rbac.ResourceFile.WithID(f.Hash).WithOwner(f.CreatedBy.String())
}
