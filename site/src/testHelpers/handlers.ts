import { rest } from "msw";
import { CreateWorkspaceBuildRequest } from "../api/typesGenerated";
import { permissionsToCheck } from "../xServices/auth/authXService";
import * as M from "./entities";
import { MockGroup, MockWorkspaceQuota } from "./entities";
import fs from "fs";
import path from "path";

export const handlers = [
  rest.get("/api/v2/templates/:templateId/daus", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockTemplateDAUResponse));
  }),

  rest.get("/api/v2/insights/daus", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockDeploymentDAUResponse));
  }),
  // Workspace proxies
  rest.get("/api/v2/regions", async (req, res, ctx) => {
    return res(
      ctx.status(200),
      ctx.json({
        regions: M.MockWorkspaceProxies,
      }),
    );
  }),
  rest.get("/api/v2/workspaceproxies", async (req, res, ctx) => {
    return res(
      ctx.status(200),
      ctx.json({
        regions: M.MockWorkspaceProxies,
      }),
    );
  }),
  // build info
  rest.get("/api/v2/buildinfo", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockBuildInfo));
  }),

  // experiments
  rest.get("/api/v2/experiments", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockExperiments));
  }),

  // update check
  rest.get("/api/v2/updatecheck", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockUpdateCheck));
  }),

  // organizations
  rest.get("/api/v2/organizations/:organizationId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockOrganization));
  }),
  rest.get(
    "api/v2/organizations/:organizationId/templates/examples",
    (req, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json([M.MockTemplateExample, M.MockTemplateExample2]),
      );
    },
  ),
  rest.get(
    "/api/v2/organizations/:organizationId/templates/:templateId",
    async (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockTemplate));
    },
  ),
  rest.get(
    "/api/v2/organizations/:organizationId/templates",
    async (req, res, ctx) => {
      return res(ctx.status(200), ctx.json([M.MockTemplate]));
    },
  ),

  // templates
  rest.get("/api/v2/templates/:templateId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockTemplate));
  }),
  rest.get("/api/v2/templates/:templateId/versions", async (req, res, ctx) => {
    return res(
      ctx.status(200),
      ctx.json([M.MockTemplateVersion2, M.MockTemplateVersion]),
    );
  }),
  rest.patch("/api/v2/templates/:templateId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockTemplate));
  }),
  rest.get(
    "/api/v2/templateversions/:templateVersionId",
    async (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockTemplateVersion));
    },
  ),
  rest.get(
    "/api/v2/templateversions/:templateVersionId/resources",
    async (req, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json([M.MockWorkspaceResource, M.MockWorkspaceResource2]),
      );
    },
  ),
  rest.get(
    "/api/v2/templateversions/:templateVersionId/rich-parameters",
    async (req, res, ctx) => {
      return res(ctx.status(200), ctx.json([]));
    },
  ),
  rest.get(
    "/api/v2/templateversions/:templateVersionId/gitauth",
    async (req, res, ctx) => {
      return res(ctx.status(200), ctx.json([]));
    },
  ),
  rest.get(
    "api/v2/organizations/:organizationId/templates/:templateName/versions/:templateVersionName",
    async (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockTemplateVersion));
    },
  ),
  rest.get(
    "api/v2/organizations/:organizationId/templates/:templateName/versions/:templateVersionName/previous",
    async (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockTemplateVersion2));
    },
  ),
  rest.delete("/api/v2/templates/:templateId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockTemplate));
  }),

  // users
  rest.get("/api/v2/users", async (req, res, ctx) => {
    return res(
      ctx.status(200),
      ctx.json({
        users: [M.MockUser, M.MockUser2, M.SuspendedMockUser],
        count: 26,
      }),
    );
  }),
  rest.post("/api/v2/users", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockUser));
  }),
  rest.get("/api/v2/users/:userid/login-type", async (req, res, ctx) => {
    return res(
      ctx.status(200),
      ctx.json({
        login_type: "password",
      }),
    );
  }),
  rest.get("/api/v2/users/me/organizations", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([M.MockOrganization]));
  }),
  rest.get(
    "/api/v2/users/me/organizations/:organizationId",
    async (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockOrganization));
    },
  ),
  rest.post("/api/v2/users/login", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockSessionToken));
  }),
  rest.post("/api/v2/users/logout", async (req, res, ctx) => {
    return res(ctx.status(200));
  }),
  rest.get("/api/v2/users/me", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockUser));
  }),
  rest.get("/api/v2/users/me/keys", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockAPIKey));
  }),
  rest.get("/api/v2/users/authmethods", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockAuthMethods));
  }),
  rest.get("/api/v2/users/roles", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockSiteRoles));
  }),
  rest.post("/api/v2/authcheck", async (req, res, ctx) => {
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

    return res(ctx.status(200), ctx.json(response));
  }),
  rest.get("/api/v2/users/:userId/gitsshkey", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockGitSSHKey));
  }),
  rest.get(
    "/api/v2/users/:userId/workspace/:workspaceName",
    async (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockWorkspace));
    },
  ),

  // First user
  rest.get("/api/v2/users/first", async (req, res, ctx) => {
    return res(ctx.status(200));
  }),
  rest.post("/api/v2/users/first", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockUser));
  }),

  // workspaces
  rest.get("/api/v2/workspaces", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockWorkspacesResponse));
  }),
  rest.get("/api/v2/workspaces/:workspaceId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockWorkspace));
  }),
  rest.put(
    "/api/v2/workspaces/:workspaceId/autostart",
    async (req, res, ctx) => {
      return res(ctx.status(200));
    },
  ),
  rest.put("/api/v2/workspaces/:workspaceId/ttl", async (req, res, ctx) => {
    return res(ctx.status(200));
  }),
  rest.put("/api/v2/workspaces/:workspaceId/extend", async (req, res, ctx) => {
    return res(ctx.status(200));
  }),

  // workspace builds
  rest.post("/api/v2/workspaces/:workspaceId/builds", async (req, res, ctx) => {
    const { transition } = req.body as CreateWorkspaceBuildRequest;
    const transitionToBuild = {
      start: M.MockWorkspaceBuild,
      stop: M.MockWorkspaceBuildStop,
      delete: M.MockWorkspaceBuildDelete,
    };
    const result = transitionToBuild[transition];
    return res(ctx.status(200), ctx.json(result));
  }),
  rest.get("/api/v2/workspaces/:workspaceId/builds", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockBuilds));
  }),
  rest.get(
    "/api/v2/users/:username/workspace/:workspaceName/builds/:buildNumber",
    (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockWorkspaceBuild));
    },
  ),
  rest.get(
    "/api/v2/workspacebuilds/:workspaceBuildId/resources",
    (req, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json([M.MockWorkspaceResource, M.MockWorkspaceResource2]),
      );
    },
  ),
  rest.patch(
    "/api/v2/workspacebuilds/:workspaceBuildId/cancel",
    (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockCancellationMessage));
    },
  ),
  rest.get(
    "/api/v2/workspacebuilds/:workspaceBuildId/logs",
    (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockWorkspaceBuildLogs));
    },
  ),
  rest.get("/api/v2/entitlements", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockEntitlements));
  }),

  // Audit
  rest.get("/api/v2/audit", (req, res, ctx) => {
    const filter = req.url.searchParams.get("q") as string;
    const logs =
      filter === "resource_type:workspace action:create"
        ? [M.MockAuditLog]
        : [M.MockAuditLog, M.MockAuditLog2];
    return res(
      ctx.status(200),
      ctx.json({
        audit_logs: logs,
        count: logs.length,
      }),
    );
  }),

  // Applications host
  rest.get("/api/v2/applications/host", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json({ host: "*.dev.coder.com" }));
  }),

  // Groups
  rest.get("/api/v2/organizations/:organizationId/groups", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([MockGroup]));
  }),

  rest.post(
    "/api/v2/organizations/:organizationId/groups",
    async (req, res, ctx) => {
      return res(ctx.status(201), ctx.json(M.MockGroup));
    },
  ),

  rest.get("/api/v2/groups/:groupId", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(MockGroup));
  }),

  rest.patch("/api/v2/groups/:groupId", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(MockGroup));
  }),

  rest.delete("/api/v2/groups/:groupId", (req, res, ctx) => {
    return res(ctx.status(204));
  }),

  rest.get("/api/v2/workspace-quota/:userId", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(MockWorkspaceQuota));
  }),

  rest.get("/api/v2/appearance", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockAppearance));
  }),

  rest.get("/api/v2/deployment/stats", (_, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockDeploymentStats));
  }),

  rest.get(
    "/api/v2/workspacebuilds/:workspaceBuildId/parameters",
    (_, res, ctx) => {
      return res(ctx.status(200), ctx.json([M.MockWorkspaceBuildParameter1]));
    },
  ),

  rest.get("/api/v2/files/:fileId", (_, res, ctx) => {
    const fileBuffer = fs.readFileSync(
      path.resolve(__dirname, "./templateFiles.tar"),
    );

    return res(
      ctx.set("Content-Length", fileBuffer.byteLength.toString()),
      ctx.set("Content-Type", "application/octet-stream"),
      // Respond with the "ArrayBuffer".
      ctx.body(fileBuffer),
    );
  }),

  rest.get(
    "/api/v2/templateversions/:templateVersionId/parameters",
    (_, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json([
          M.MockTemplateVersionParameter1,
          M.MockTemplateVersionParameter2,
          M.MockTemplateVersionParameter3,
        ]),
      );
    },
  ),

  rest.get(
    "/api/v2/templateversions/:templateVersionId/variables",
    (_, res, ctx) => {
      return res(
        ctx.status(200),
        ctx.json([
          M.MockTemplateVersionVariable1,
          M.MockTemplateVersionVariable2,
          M.MockTemplateVersionVariable3,
        ]),
      );
    },
  ),

  rest.get("/api/v2/deployment/ssh", (_, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockDeploymentSSH));
  }),

  rest.get("/api/v2/workspaceagents/:agent/logs", (_, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockWorkspaceAgentLogs));
  }),

  rest.get("/api/v2/debug/health", (_, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockHealth));
  }),

  rest.get("/api/v2/workspaceagents/:agent/listening-ports", (_, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockListeningPortsResponse));
  }),
];
