import fs from "fs";
import { http, HttpResponse } from "msw";
import path from "path";
import type { CreateWorkspaceBuildRequest } from "api/typesGenerated";
import { permissionsToCheck } from "contexts/auth/permissions";
import * as M from "./entities";
import { MockGroup, MockWorkspaceQuota } from "./entities";

export const handlers = [
  http.get("/api/v2/templates/:templateId/daus", async () => {
    return HttpResponse.json(M.MockTemplateDAUResponse);
  }),

  http.get("/api/v2/insights/daus", async () => {
    return HttpResponse.json(M.MockDeploymentDAUResponse);
  }),
  // Workspace proxies
  http.get("/api/v2/regions", async () => {
    return HttpResponse.json({
      regions: M.MockWorkspaceProxies,
    });
  }),
  http.get("/api/v2/workspaceproxies", async () => {
    return HttpResponse.json({
      regions: M.MockWorkspaceProxies,
    });
  }),
  // build info
  http.get("/api/v2/buildinfo", async () => {
    return HttpResponse.json(M.MockBuildInfo);
  }),

  // experiments
  http.get("/api/v2/experiments", async () => {
    return HttpResponse.json(M.MockExperiments);
  }),

  // update check
  http.get("/api/v2/updatecheck", async () => {
    return HttpResponse.json(M.MockUpdateCheck);
  }),

  // organizations
  http.get("/api/v2/organizations/:organizationId", async () => {
    return HttpResponse.json(M.MockOrganization);
  }),
  http.get("api/v2/organizations/:organizationId/templates/examples", () => {
    return HttpResponse.json([M.MockTemplateExample, M.MockTemplateExample2]);
  }),
  http.get(
    "/api/v2/organizations/:organizationId/templates/:templateId",
    async () => {
      return HttpResponse.json(M.MockTemplate);
    },
  ),
  http.get("/api/v2/organizations/:organizationId/templates", async () => {
    return HttpResponse.json([M.MockTemplate]);
  }),

  // templates
  http.get("/api/v2/templates/:templateId", async () => {
    return HttpResponse.json(M.MockTemplate);
  }),
  http.get("/api/v2/templates/:templateId/versions", async () => {
    return HttpResponse.json([M.MockTemplateVersion2, M.MockTemplateVersion]);
  }),
  http.patch("/api/v2/templates/:templateId/versions", async () => {
    return new HttpResponse(null, { status: 200 });
  }),
  http.patch("/api/v2/templates/:templateId", async () => {
    return HttpResponse.json(M.MockTemplate);
  }),
  http.get("/api/v2/templateversions/:templateVersionId", async () => {
    return HttpResponse.json(M.MockTemplateVersion);
  }),
  http.get(
    "/api/v2/templateversions/:templateVersionId/resources",
    async () => {
      return HttpResponse.json([
        M.MockWorkspaceResource,
        M.MockWorkspaceVolumeResource,
        M.MockWorkspaceImageResource,
        M.MockWorkspaceContainerResource,
      ]);
    },
  ),
  http.get(
    "/api/v2/templateversions/:templateVersionId/rich-parameters",
    async () => {
      return HttpResponse.json([]);
    },
  ),
  http.get(
    "/api/v2/templateversions/:templateVersionId/external-auth",
    async () => {
      return HttpResponse.json([]);
    },
  ),
  http.get("/api/v2/templateversions/:templateversionId/logs", async () => {
    return HttpResponse.json(M.MockWorkspaceBuildLogs);
  }),
  http.get(
    "api/v2/organizations/:organizationId/templates/:templateName/versions/:templateVersionName",
    async () => {
      return HttpResponse.json(M.MockTemplateVersion);
    },
  ),
  http.get(
    "api/v2/organizations/:organizationId/templates/:templateName/versions/:templateVersionName/previous",
    async () => {
      return HttpResponse.json(M.MockTemplateVersion2);
    },
  ),
  http.delete("/api/v2/templates/:templateId", async () => {
    return HttpResponse.json(M.MockTemplate);
  }),

  // users
  http.get("/api/v2/users", async () => {
    return HttpResponse.json({
      users: [M.MockUser, M.MockUser2, M.SuspendedMockUser],
      count: 26,
    });
  }),
  http.post("/api/v2/users", async () => {
    return HttpResponse.json(M.MockUser);
  }),
  http.get("/api/v2/users/:userid/login-type", async () => {
    return HttpResponse.json({
      login_type: "password",
    });
  }),
  http.get("/api/v2/users/me/organizations", () => {
    return HttpResponse.json([M.MockOrganization]);
  }),
  http.get("/api/v2/users/me/organizations/:organizationId", async () => {
    return HttpResponse.json(M.MockOrganization);
  }),
  http.post("/api/v2/users/login", async () => {
    return HttpResponse.json(M.MockSessionToken);
  }),
  http.post("/api/v2/users/logout", async () => {
    return new HttpResponse(null, { status: 200 });
  }),
  http.get("/api/v2/users/me", async () => {
    return HttpResponse.json(M.MockUser);
  }),
  http.get("/api/v2/users/me/keys", async () => {
    return HttpResponse.json(M.MockAPIKey);
  }),
  http.get("/api/v2/users/authmethods", async () => {
    return HttpResponse.json(M.MockAuthMethodsPasswordOnly);
  }),
  http.get("/api/v2/users/roles", async () => {
    return HttpResponse.json(M.MockSiteRoles);
  }),
  http.post("/api/v2/authcheck", async () => {
    const permissions = [
      ...Object.keys(permissionsToCheck),
      "canUpdateTemplate",
      "updateWorkspace",
    ];
    const response = permissions.reduce((obj, permission) => {
      return {
        ...obj,
        [permission]: true,
      };
    }, {});

    return HttpResponse.json(response);
  }),
  http.get("/api/v2/users/:userId/gitsshkey", async () => {
    return HttpResponse.json(M.MockGitSSHKey);
  }),
  http.get("/api/v2/users/:userId/workspace/:workspaceName", async () => {
    return HttpResponse.json(M.MockWorkspace);
  }),

  // First user
  http.get("/api/v2/users/first", async () => {
    return new HttpResponse(null, { status: 200 });
  }),
  http.post("/api/v2/users/first", async () => {
    return HttpResponse.json(M.MockUser);
  }),

  // workspaces
  http.get("/api/v2/workspaces", async () => {
    return HttpResponse.json(M.MockWorkspacesResponse);
  }),
  http.get("/api/v2/workspaces/:workspaceId", async () => {
    return HttpResponse.json(M.MockWorkspace);
  }),
  http.put("/api/v2/workspaces/:workspaceId/autostart", async () => {
    return new HttpResponse(null, { status: 200 });
  }),
  http.put("/api/v2/workspaces/:workspaceId/ttl", async () => {
    return new HttpResponse(null, { status: 200 });
  }),
  http.put("/api/v2/workspaces/:workspaceId/extend", async () => {
    return new HttpResponse(null, { status: 200 });
  }),
  http.get("/api/v2/workspaces/:workspaceId/resolve-autostart", async () => {
    return HttpResponse.json({ parameter_mismatch: false });
  }),

  // workspace builds
  http.post("/api/v2/workspaces/:workspaceId/builds", async ({ request }) => {
    const { transition } =
      (await request.json()) as CreateWorkspaceBuildRequest;
    const transitionToBuild = {
      start: M.MockWorkspaceBuild,
      stop: M.MockWorkspaceBuildStop,
      delete: M.MockWorkspaceBuildDelete,
    };
    const result = transitionToBuild[transition];
    return HttpResponse.json(result);
  }),
  http.get("/api/v2/workspaces/:workspaceId/builds", async () => {
    return HttpResponse.json(M.MockBuilds);
  }),
  http.get(
    "/api/v2/users/:username/workspace/:workspaceName/builds/:buildNumber",
    () => {
      return HttpResponse.json(M.MockWorkspaceBuild);
    },
  ),
  http.get("/api/v2/workspacebuilds/:workspaceBuildId/resources", () => {
    return HttpResponse.json([
      M.MockWorkspaceResource,
      M.MockWorkspaceVolumeResource,
      M.MockWorkspaceImageResource,
      M.MockWorkspaceContainerResource,
    ]);
  }),
  http.patch("/api/v2/workspacebuilds/:workspaceBuildId/cancel", () => {
    return HttpResponse.json(M.MockCancellationMessage);
  }),
  http.get("/api/v2/workspacebuilds/:workspaceBuildId/logs", () => {
    return HttpResponse.json(M.MockWorkspaceBuildLogs);
  }),
  http.get("/api/v2/entitlements", () => {
    return HttpResponse.json(M.MockEntitlements);
  }),

  // Audit
  http.get("/api/v2/audit", ({ request }) => {
    const { searchParams } = new URL(request.url);
    const filter = searchParams.get("q") as string;
    const logs =
      filter === "resource_type:workspace action:create"
        ? [M.MockAuditLog]
        : [M.MockAuditLog, M.MockAuditLog2];
    return HttpResponse.json({
      audit_logs: logs,
      count: logs.length,
    });
  }),

  // Applications host
  http.get("/api/v2/applications/host", () => {
    return HttpResponse.json({ host: "*.dev.coder.com" });
  }),

  // Groups
  http.get("/api/v2/organizations/:organizationId/groups", () => {
    return HttpResponse.json([MockGroup]);
  }),

  http.post("/api/v2/organizations/:organizationId/groups", async () => {
    return HttpResponse.json(M.MockGroup, { status: 201 });
  }),

  http.get("/api/v2/groups/:groupId", () => {
    return HttpResponse.json(MockGroup);
  }),

  http.patch("/api/v2/groups/:groupId", () => {
    return HttpResponse.json(MockGroup);
  }),

  http.delete("/api/v2/groups/:groupId", () => {
    return new HttpResponse(null, { status: 204 });
  }),

  http.get("/api/v2/workspace-quota/:userId", () => {
    return HttpResponse.json(MockWorkspaceQuota);
  }),

  http.get("/api/v2/appearance", () => {
    return HttpResponse.json(M.MockAppearanceConfig);
  }),

  http.get("/api/v2/deployment/stats", () => {
    return HttpResponse.json(M.MockDeploymentStats);
  }),

  http.get("/api/v2/deployment/config", () => {
    return HttpResponse.json(M.MockDeploymentConfig);
  }),

  http.get("/api/v2/workspacebuilds/:workspaceBuildId/parameters", () => {
    return HttpResponse.json([
      M.MockWorkspaceBuildParameter1,
      M.MockWorkspaceBuildParameter2,
      M.MockWorkspaceBuildParameter3,
      M.MockWorkspaceBuildParameter4,
      M.MockWorkspaceBuildParameter5,
    ]);
  }),

  http.post("/api/v2/files", () => {
    return HttpResponse.json({
      hash: "some-file-hash",
    });
  }),

  http.get("/api/v2/files/:fileId", () => {
    const fileBuffer = fs.readFileSync(
      path.resolve(__dirname, "./templateFiles.tar"),
    );

    return HttpResponse.arrayBuffer(fileBuffer);
  }),

  http.get("/api/v2/templateversions/:templateVersionId/parameters", () => {
    return HttpResponse.json([
      M.MockTemplateVersionParameter1,
      M.MockTemplateVersionParameter2,
      M.MockTemplateVersionParameter3,
    ]);
  }),

  http.get("/api/v2/templateversions/:templateVersionId/variables", () => {
    return HttpResponse.json([
      M.MockTemplateVersionVariable1,
      M.MockTemplateVersionVariable2,
      M.MockTemplateVersionVariable3,
    ]);
  }),

  http.get("/api/v2/deployment/ssh", () => {
    return HttpResponse.json(M.MockDeploymentSSH);
  }),

  http.get("/api/v2/workspaceagents/:agent/logs", () => {
    return HttpResponse.json(M.MockWorkspaceAgentLogs);
  }),

  http.get("/api/v2/debug/health", () => {
    return HttpResponse.json(M.MockHealth);
  }),

  http.get("/api/v2/workspaceagents/:agent/listening-ports", () => {
    return HttpResponse.json(M.MockListeningPortsResponse);
  }),

  http.get("/api/v2/integrations/jfrog/xray-scan", () => {
    return new HttpResponse(null, { status: 404 });
  }),
];
