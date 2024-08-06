export const checks = {
  viewAllUsers: "viewAllUsers",
  updateUsers: "updateUsers",
  createUser: "createUser",
  createTemplates: "createTemplates",
  updateTemplates: "updateTemplates",
  deleteTemplates: "deleteTemplates",
  viewAnyAuditLog: "viewAnyAuditLog",
  viewDeploymentValues: "viewDeploymentValues",
  editDeploymentValues: "editDeploymentValues",
  viewUpdateCheck: "viewUpdateCheck",
  viewExternalAuthConfig: "viewExternalAuthConfig",
  viewDeploymentStats: "viewDeploymentStats",
  editWorkspaceProxies: "editWorkspaceProxies",
  createOrganization: "createOrganization",
  editAnyOrganization: "editAnyOrganization",
  viewAnyGroup: "viewAnyGroup",
  createGroup: "createGroup",
  viewAllLicenses: "viewAllLicenses",
} as const;

export const permissionsToCheck = {
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
  [checks.createTemplates]: {
    object: {
      resource_type: "template",
    },
    action: "update",
  },
  [checks.updateTemplates]: {
    object: {
      resource_type: "template",
    },
    action: "update",
  },
  [checks.deleteTemplates]: {
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
  [checks.editDeploymentValues]: {
    object: {
      resource_type: "deployment_config",
    },
    action: "update",
  },
  [checks.viewUpdateCheck]: {
    object: {
      resource_type: "deployment_config",
    },
    action: "read",
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
  [checks.createOrganization]: {
    object: {
      resource_type: "organization",
    },
    action: "create",
  },
  [checks.editAnyOrganization]: {
    object: {
      resource_type: "organization",
      any_org: true,
    },
    action: "update",
  },
  [checks.viewAnyGroup]: {
    object: {
      resource_type: "group",
      org_id: "any",
    },
    action: "read",
  },
  [checks.createGroup]: {
    object: {
      resource_type: "group",
    },
    action: "create",
  },
  [checks.viewAllLicenses]: {
    object: {
      resource_type: "license",
    },
    action: "read",
  },
} as const;

export type Permissions = Record<keyof typeof permissionsToCheck, boolean>;
