import type { AuthorizationCheck } from "api/typesGenerated";

// checks language should include either "any" or "all" in the name.
// "any" means the actor has permission to do the action on at least 1 resource
// in at least 1 organization. So an org template admin can create a template on
// at least 1 org, so "createAnyTemplate" returns "true".
// "all" requires the actor to have the permission across all organizations.
// So an "org template admin" would fail "createAllTemplates".
//
// Any check not using this language should be updated to use it.
export const checks = {
  viewAllUsers: "viewAllUsers",
  updateUsers: "updateUsers",
  createUser: "createUser",
  createAnyTemplate: "createAnyTemplate",
  updateAllTemplates: "updateAllTemplates",
  deleteAllTemplates: "deleteAllTemplates",
  viewAnyAuditLog: "viewAnyAuditLog",
  viewDeploymentValues: "viewDeploymentValues",
  createAnyGroup: "createAnyGroup",
  viewExternalAuthConfig: "viewExternalAuthConfig",
  updateDeploymentConfig: "updateDeploymentConfig",
  viewDeploymentStats: "viewDeploymentStats",
  editWorkspaceProxies: "editWorkspaceProxies",
  viewAllLicenses: "viewAllLicenses",
} as const;

export const permissionsToCheck: Record<
  keyof typeof checks,
  AuthorizationCheck
> = {
  [checks.viewAllLicenses]: {
    object: {
      resource_type: "license",
    },
    action: "read",
  },
  [checks.viewAllUsers]: {
    object: {
      resource_type: "user",
    },
    action: "read",
  },
  [checks.updateUsers]: {
    object: {
      resource_type: "user",
    },
    action: "update",
  },
  [checks.createUser]: {
    object: {
      resource_type: "user",
    },
    action: "create",
  },
  [checks.createAnyTemplate]: {
    object: {
      resource_type: "template",
      any_org: true,
    },
    action: "update",
  },
  [checks.updateAllTemplates]: {
    object: {
      resource_type: "template",
    },
    action: "update",
  },
  [checks.deleteAllTemplates]: {
    object: {
      resource_type: "template",
    },
    action: "delete",
  },
  [checks.viewAnyAuditLog]: {
    object: {
      resource_type: "audit_log",
      any_org: true,
    },
    action: "read",
  },
  [checks.viewDeploymentValues]: {
    object: {
      resource_type: "deployment_config",
    },
    action: "read",
  },
  [checks.updateDeploymentConfig]: {
    object: {
      resource_type: "deployment_config",
    },
    action: "update",
  },
  [checks.createAnyGroup]: {
    object: {
      resource_type: "group",
      any_org: true,
    },
    action: "create",
  },
  [checks.viewExternalAuthConfig]: {
    object: {
      resource_type: "deployment_config",
    },
    action: "read",
  },
  [checks.viewDeploymentStats]: {
    object: {
      resource_type: "deployment_stats",
    },
    action: "read",
  },
  [checks.editWorkspaceProxies]: {
    object: {
      resource_type: "workspace_proxy",
    },
    action: "create",
  },
} as const;

export type Permissions = Record<keyof typeof permissionsToCheck, boolean>;
