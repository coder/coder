import { http, HttpResponse } from "msw";
import { CreateWorkspaceBuildRequest } from "api/typesGenerated";
import { permissionsToCheck } from "components/AuthProvider/permissions";
import * as M from "./entities";
import fs from "fs";
import path from "path";

export const handlers = [
  // http.get("/api/v2/templates/:templateId/daus", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockTemplateDAUResponse));
  // }),

  http.get("/api/v2/templates/:templateId/daus", () => {
    return HttpResponse.json(M.MockTemplateDAUResponse, { status: 200 });
  }),

  // http.get("/api/v2/insights/daus", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockDeploymentDAUResponse));
  // }),

  http.get("/api/v2/insights/daus", () => {
    return HttpResponse.json(M.MockDeploymentDAUResponse, { status: 200 });
  }),
  // Workspace proxies
  // http.get("/api/v2/regions", async (req, res, ctx) => {
  //   return res(
  //     ctx.status(200),
  //     ctx.json({
  //       regions: M.MockWorkspaceProxies,
  //     }),
  //   );
  // }),
  http.get("/api/v2/regions", () => {
    return HttpResponse.json(M.MockWorkspaceProxies, { status: 200 });
  }),
  // http.get("/api/v2/workspaceproxies", async (req, res, ctx) => {
  //   return res(
  //     ctx.status(200),
  //     ctx.json({
  //       regions: M.MockWorkspaceProxies,
  //     }),
  //   );
  // }),
  http.get("/api/v2/workspaceproxies", () => {
    return HttpResponse.json(M.MockWorkspaceProxies, { status: 200 });
  }),
  // build info
  // http.get("/api/v2/buildinfo", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockBuildInfo));
  // }),
  http.get("/api/v2/buildinfo", () => {
    return HttpResponse.json(M.MockBuildInfo, { status: 200 });
  }),
  // experiments
  // http.get("/api/v2/experiments", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockExperiments));
  // }),
  http.get("/api/v2/experiments", () => {
    return HttpResponse.json(M.MockExperiments, { status: 200 });
  }),
  // http.get("/api/v2/entitlements", (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockEntitlements));
  // }),
  http.get("/api/v2/entitlements", () => {
    return HttpResponse.json(M.MockEntitlements, { status: 200 });
  }),
  // update check
  // http.get("/api/v2/updatecheck", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockUpdateCheck));
  // }),
  http.get("/api/v2/updatecheck", () => {
    return HttpResponse.json(M.MockUpdateCheck, { status: 200 });
  }),
  // organizations
  // http.get("/api/v2/organizations/:organizationId", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockOrganization));
  // }),
  http.get("/api/v2/organizations/:organizationId", () => {
    return HttpResponse.json(M.MockOrganization, { status: 200 });
  }),
  // http.get(
  //   "api/v2/organizations/:organizationId/templates/examples",
  //   (req, res, ctx) => {
  //     return res(
  //       ctx.status(200),
  //       ctx.json([M.MockTemplateExample, M.MockTemplateExample2]),
  //     );
  //   },
  // ),
  http.get("api/v2/organizations/:organizationId/templates/examples", () => {
    return HttpResponse.json([M.MockTemplateExample, M.MockTemplateExample2], {
      status: 200,
    });
  }),

  // http.get(
  //   "/api/v2/organizations/:organizationId/templates/:templateId",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockTemplate));
  //   },
  // ),
  http.get(
    "/api/v2/organizations/:organizationId/templates/:templateId",
    () => {
      return HttpResponse.json(M.MockTemplate, { status: 200 });
    },
  ),
  // http.get(
  //   "/api/v2/organizations/:organizationId/templates",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json([M.MockTemplate]));
  //   },
  // ),

  http.get("/api/v2/organizations/:organizationId/templates", () => {
    return HttpResponse.json([M.MockTemplate], { status: 200 });
  }),

  // templates
  // http.get("/api/v2/templates/:templateId", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockTemplate));
  // }),
  http.get("/api/v2/templates/:templateId", () => {
    return HttpResponse.json(M.MockTemplate, { status: 200 });
  }),
  // http.get("/api/v2/templates/:templateId/versions", async (req, res, ctx) => {
  //   return res(
  //     ctx.status(200),
  //     ctx.json([M.MockTemplateVersion2, M.MockTemplateVersion]),
  //   );
  // }),
  http.get("/api/v2/templates/:templateId/versions", () => {
    return HttpResponse.json([M.MockTemplateVersion2, M.MockTemplateVersion], {
      status: 200,
    });
  }),
  // http.patch("/api/v2/templates/:templateId", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockTemplate));
  // }),
  http.patch("/api/v2/templates/:templateId", () => {
    return HttpResponse.json(M.MockTemplate, { status: 200 });
  }),
  // http.get(
  //   "/api/v2/templateversions/:templateVersionId",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockTemplateVersion));
  //   },
  // ),
  http.get("/api/v2/templateversions/:templateVersionId", () => {
    return HttpResponse.json(M.MockTemplateVersion, { status: 200 });
  }),
  // http.get(
  //   "/api/v2/templateversions/:templateVersionId/resources",
  //   async (req, res, ctx) => {
  //     return res(
  //       ctx.status(200),
  //       ctx.json([
  //         M.MockWorkspaceResource,
  //         M.MockWorkspaceVolumeResource,
  //         M.MockWorkspaceImageResource,
  //         M.MockWorkspaceContainerResource,
  //       ]),
  //     );
  //   },
  // ),
  http.get("/api/v2/templateversions/:templateVersionId/resources", () => {
    return HttpResponse.json(
      [
        M.MockWorkspaceResource,
        M.MockWorkspaceVolumeResource,
        M.MockWorkspaceImageResource,
        M.MockWorkspaceContainerResource,
      ],
      { status: 200 },
    );
  }),
  // http.get(
  //   "/api/v2/templateversions/:templateVersionId/rich-parameters",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json([]));
  //   },
  // ),
  http.get(
    "/api/v2/templateversions/:templateVersionId/rich-parameters",
    () => {
      return HttpResponse.json([], { status: 200 });
    },
  ),
  // http.get(
  //   "/api/v2/templateversions/:templateVersionId/external-auth",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json([]));
  //   },
  // ),
  http.get("/api/v2/templateversions/:templateVersionId/external-auth", () => {
    return HttpResponse.json([], { status: 200 });
  }),
  // http.get(
  //   "/api/v2/templateversions/:templateversionId/logs",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockWorkspaceBuildLogs));
  //   },
  // ),
  http.get("/api/v2/templateversions/:templateversionId/logs", () => {
    return HttpResponse.json(M.MockWorkspaceBuildLogs, { status: 200 });
  }),
  // http.get(
  //   "api/v2/organizations/:organizationId/templates/:templateName/versions/:templateVersionName",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockTemplateVersion));
  //   },
  // ),
  http.get(
    "api/v2/organizations/:organizationId/templates/:templateName/versions/:templateVersionName",
    () => {
      return HttpResponse.json(M.MockTemplateVersion, { status: 200 });
    },
  ),
  // http.get(
  //   "api/v2/organizations/:organizationId/templates/:templateName/versions/:templateVersionName/previous",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockTemplateVersion2));
  //   },
  // ),
  http.get(
    "api/v2/organizations/:organizationId/templates/:templateName/versions/:templateVersionName/previous",
    () => {
      return HttpResponse.json(M.MockTemplateVersion2, { status: 200 });
    },
  ),
  // http.delete("/api/v2/templates/:templateId", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockTemplate));
  // }),

  http.delete("/api/v2/templates/:templateId", () => {
    return HttpResponse.json(M.MockTemplate, { status: 200 });
  }),

  // users
  // http.get("/api/v2/users", async (req, res, ctx) => {
  //   return res(
  //     ctx.status(200),
  //     ctx.json({
  //       users: [M.MockUser, M.MockUser2, M.SuspendedMockUser],
  //       count: 26,
  //     }),
  //   );
  // }),
  http.get("/api/v2/users", () => {
    return HttpResponse.json(
      {
        users: [M.MockUser, M.MockUser2, M.SuspendedMockUser],
        count: 26,
      },
      { status: 200 },
    );
  }),

  // http.post("/api/v2/users", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockUser));
  // }),
  http.post("/api/v2/users", () => {
    return HttpResponse.json(M.MockUser, { status: 200 });
  }),
  // http.get("/api/v2/users/:userid/login-type", async (req, res, ctx) => {
  //   return res(
  //     ctx.status(200),
  //     ctx.json({
  //       login_type: "password",
  //     }),
  //   );
  // }),
  http.get("/api/v2/users/:userid/login-type", () => {
    return HttpResponse.json(
      {
        login_type: "password",
      },
      { status: 200 },
    );
  }),
  // http.get("/api/v2/users/me/organizations", (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json([M.MockOrganization]));
  // }),
  http.get("/api/v2/users/me/organizations", () => {
    return HttpResponse.json([M.MockOrganization], { status: 200 });
  }),
  // http.get(
  //   "/api/v2/users/me/organizations/:organizationId",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockOrganization));
  //   },
  // ),
  http.get("/api/v2/users/me/organizations/:organizationId", () => {
    return HttpResponse.json(M.MockOrganization, { status: 200 });
  }),
  // http.post("/api/v2/users/login", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockSessionToken));
  // }),
  http.post("/api/v2/users/login", () => {
    return HttpResponse.json(M.MockSessionToken, { status: 200 });
  }),
  // http.post("/api/v2/users/logout", async (req, res, ctx) => {
  //   return res(ctx.status(200));
  // }),
  http.post("/api/v2/users/logout", () => {
    return HttpResponse.json(null, { status: 200 });
  }),
  // http.get("/api/v2/users/me", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockUser));
  // }),
  http.get("/api/v2/users/me", () => {
    return HttpResponse.json(M.MockUser, { status: 200 });
  }),
  // http.get("/api/v2/users/me/keys", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockAPIKey));
  // }),
  http.get("/api/v2/users/me/keys", () => {
    return HttpResponse.json(M.MockAPIKey, { status: 200 });
  }),
  // http.get("/api/v2/users/authmethods", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockAuthMethods));
  // }),
  http.get("/api/v2/users/authmethods", () => {
    return HttpResponse.json(M.MockAuthMethods, { status: 200 });
  }),
  // http.get("/api/v2/users/roles", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockSiteRoles));
  // }),
  http.get("/api/v2/users/roles", () => {
    return HttpResponse.json(M.MockSiteRoles, { status: 200 });
  }),
  // http.post("/api/v2/authcheck", async (req, res, ctx) => {
  //   const permissions = [
  //     ...Object.keys(permissionsToCheck),
  //     "canUpdateTemplate",
  //     "updateWorkspace",
  //   ];
  //   const response = permissions.reduce((obj, permission) => {
  //     return {
  //       ...obj,
  //       [permission]: true,
  //     };
  //   }, {});

  //   return res(ctx.status(200), ctx.json(response));
  // }),
  http.post("/api/v2/authcheck", () => {
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
    return HttpResponse.json(response, { status: 200 });
  }),
  // http.get("/api/v2/users/:userId/gitsshkey", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockGitSSHKey));
  // }),
  http.get("/api/v2/users/:userId/gitsshkey", () => {
    return HttpResponse.json(M.MockGitSSHKey, { status: 200 });
  }),
  // http.get(
  //   "/api/v2/users/:userId/workspace/:workspaceName",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockWorkspace));
  //   },
  // ),
  http.get("/api/v2/users/:userId/workspace/:workspaceName", () => {
    return HttpResponse.json(M.MockWorkspace, { status: 200 });
  }),

  // http.get("/api/v2/users/first", async (req, res, ctx) => {
  //   return res(ctx.status(200));
  // }),
  http.get("/api/v2/users/first", () => {
    return HttpResponse.json(null, { status: 200 });
  }),
  // http.post("/api/v2/users/first", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockUser));
  // }),
  http.post("/api/v2/users/first", () => {
    return HttpResponse.json(M.MockUser, { status: 200 });
  }),

  // workspaces
  // http.get("/api/v2/workspaces", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockWorkspacesResponse));
  // }),
  http.get("/api/v2/workspaces", () => {
    return HttpResponse.json(M.MockWorkspacesResponse, { status: 200 });
  }),
  // http.get("/api/v2/workspaces/:workspaceId", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockWorkspace));
  // }),
  http.get("/api/v2/workspaces/:workspaceId", () => {
    return HttpResponse.json(M.MockWorkspace, { status: 200 });
  }),
  // http.put(
  //   "/api/v2/workspaces/:workspaceId/autostart",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(200));
  //   },
  // ),
  http.put("/api/v2/workspaces/:workspaceId/autostart", () => {
    return HttpResponse.json(null, { status: 200 });
  }),
  // http.put("/api/v2/workspaces/:workspaceId/ttl", async (req, res, ctx) => {
  //   return res(ctx.status(200));
  // }),
  http.put("/api/v2/workspaces/:workspaceId/ttl", () => {
    return HttpResponse.json(null, { status: 200 });
  }),
  // http.put("/api/v2/workspaces/:workspaceId/extend", async (req, res, ctx) => {
  //   return res(ctx.status(200));
  // }),
  http.put("/api/v2/workspaces/:workspaceId/extend", () => {
    return HttpResponse.json(null, { status: 200 });
  }),

  // workspace builds
  // http.post("/api/v2/workspaces/:workspaceId/builds", async (req, res, ctx) => {
  //   const { transition } = req.body as CreateWorkspaceBuildRequest;
  //   const transitionToBuild = {
  //     start: M.MockWorkspaceBuild,
  //     stop: M.MockWorkspaceBuildStop,
  //     delete: M.MockWorkspaceBuildDelete,
  //   };
  //   const result = transitionToBuild[transition];
  //   return res(ctx.status(200), ctx.json(result));
  // }),
  http.post("/api/v2/workspaces/:workspaceId/builds", ({ request }) => {
    const { transition } =
      request.body as unknown as CreateWorkspaceBuildRequest;
    const transitionToBuild = {
      start: M.MockWorkspaceBuild,
      stop: M.MockWorkspaceBuildStop,
      delete: M.MockWorkspaceBuildDelete,
    };
    const response = transitionToBuild[transition];
    return HttpResponse.json(response, { status: 200 });
  }),
  // http.get("/api/v2/workspaces/:workspaceId/builds", async (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockBuilds));
  // }),
  http.get("/api/v2/workspaces/:workspaceId/builds", () => {
    return HttpResponse.json(M.MockBuilds, { status: 200 });
  }),
  // http.get(
  //   "/api/v2/users/:username/workspace/:workspaceName/builds/:buildNumber",
  //   (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockWorkspaceBuild));
  //   },
  // ),
  http.get(
    "/api/v2/users/:username/workspace/:workspaceName/builds/:buildNumber",
    () => {
      return HttpResponse.json(M.MockWorkspaceBuild, { status: 200 });
    },
  ),
  // http.get(
  //   "/api/v2/workspacebuilds/:workspaceBuildId/resources",
  //   (req, res, ctx) => {
  //     return res(
  //       ctx.status(200),
  //       ctx.json([
  //         M.MockWorkspaceResource,
  //         M.MockWorkspaceVolumeResource,
  //         M.MockWorkspaceImageResource,
  //         M.MockWorkspaceContainerResource,
  //       ]),
  //     );
  //   },
  // ),
  http.get("/api/v2/workspacebuilds/:workspaceBuildId/resources", () => {
    return HttpResponse.json(
      [
        M.MockWorkspaceResource,
        M.MockWorkspaceVolumeResource,
        M.MockWorkspaceImageResource,
        M.MockWorkspaceContainerResource,
      ],
      { status: 200 },
    );
  }),
  // http.patch(
  //   "/api/v2/workspacebuilds/:workspaceBuildId/cancel",
  //   (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockCancellationMessage));
  //   },
  // ),
  http.patch("/api/v2/workspacebuilds/:workspaceBuildId/cancel", () => {
    return HttpResponse.json(M.MockCancellationMessage, { status: 200 });
  }),
  // http.get(
  //   "/api/v2/workspacebuilds/:workspaceBuildId/logs",
  //   (req, res, ctx) => {
  //     return res(ctx.status(200), ctx.json(M.MockWorkspaceBuildLogs));
  //   },
  // ),
  http.get("/api/v2/workspacebuilds/:workspaceBuildId/logs", () => {
    return HttpResponse.json(M.MockWorkspaceBuildLogs, { status: 200 });
  }),

  // Audit
  // http.get("/api/v2/audit", (req, res, ctx) => {
  //   const filter = req.url.searchParams.get("q") as string;
  //   const logs =
  //     filter === "resource_type:workspace action:create"
  //       ? [M.MockAuditLog]
  //       : [M.MockAuditLog, M.MockAuditLog2];
  //   return res(
  //     ctx.status(200),
  //     ctx.json({
  //       audit_logs: logs,
  //       count: logs.length,
  //     }),
  //   );
  // }),
  http.get("/api/v2/audit", ({ request }) => {
    const searchParams = new URL(request.url).searchParams;
    const filter = searchParams.get("q") as string;
    const logs =
      filter === "resource_type:workspace action:create"
        ? [M.MockAuditLog]
        : [M.MockAuditLog, M.MockAuditLog2];
    return HttpResponse.json(
      {
        audit_logs: logs,
        count: logs.length,
      },
      { status: 200 },
    );
  }),

  // Applications host
  // http.get("/api/v2/applications/host", (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json({ host: "*.dev.coder.com" }));
  // }),
  http.get("/api/v2/applications/host", () => {
    return HttpResponse.json({ host: "*.dev.coder.com" }, { status: 200 });
  }),
  // Groups
  // http.get("/api/v2/organizations/:organizationId/groups", (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json([MockGroup]));
  // }),
  http.get("/api/v2/organizations/:organizationId/groups", () => {
    return HttpResponse.json([M.MockGroup], { status: 200 });
  }),
  // http.post(
  //   "/api/v2/organizations/:organizationId/groups",
  //   async (req, res, ctx) => {
  //     return res(ctx.status(201), ctx.json(M.MockGroup));
  //   },
  // ),
  http.post("/api/v2/organizations/:organizationId/groups", () => {
    return HttpResponse.json(M.MockGroup, { status: 200 });
  }),
  // http.get("/api/v2/groups/:groupId", (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockGroup));
  // }),
  http.get("/api/v2/groups/:groupId", () => {
    return HttpResponse.json(M.MockGroup, { status: 200 });
  }),

  // http.patch("/api/v2/groups/:groupId", (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockGroup));
  // }),
  http.patch("/api/v2/groups/:groupId", () => {
    return HttpResponse.json(M.MockGroup, { status: 200 });
  }),

  // http.delete("/api/v2/groups/:groupId", (req, res, ctx) => {
  //   return res(ctx.status(204));
  // }),
  http.delete("/api/v2/groups/:groupId", () => {
    return HttpResponse.json(null, { status: 204 });
  }),

  // http.get("/api/v2/workspace-quota/:userId", (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(MockWorkspaceQuota));
  // }),
  http.get("/api/v2/workspace-quota/:userId", () => {
    return HttpResponse.json(M.MockWorkspaceQuota, { status: 200 });
  }),

  // http.get("/api/v2/appearance", (req, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockAppearanceConfig));
  // }),
  http.get("/api/v2/appearance", () => {
    return HttpResponse.json(M.MockAppearanceConfig, { status: 200 });
  }),

  // http.get("/api/v2/deployment/stats", (_, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockDeploymentStats));
  // }),
  http.get("/api/v2/deployment/stats", () => {
    return HttpResponse.json(M.MockDeploymentStats, { status: 200 });
  }),

  // http.get("/api/v2/deployment/config", (_, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockDeploymentConfig));
  // }),
  http.get("/api/v2/deployment/config", () => {
    return HttpResponse.json(M.MockDeploymentConfig, { status: 200 });
  }),

  // http.get(
  //   "/api/v2/workspacebuilds/:workspaceBuildId/parameters",
  //   (_, res, ctx) => {
  //     return res(ctx.status(200), ctx.json([M.MockWorkspaceBuildParameter1]));
  //   },
  // ),
  http.get("/api/v2/workspacebuilds/:workspaceBuildId/parameters", () => {
    return HttpResponse.json([M.MockWorkspaceBuildParameter1], { status: 200 });
  }),

  // http.get("/api/v2/files/:fileId", (_, res, ctx) => {
  //   const fileBuffer = fs.readFileSync(
  //     path.resolve(__dirname, "./templateFiles.tar"),
  //   );

  //   return res(
  //     ctx.set("Content-Length", fileBuffer.byteLength.toString()),
  //     ctx.set("Content-Type", "application/octet-stream"),
  //     // Respond with the "ArrayBuffer".
  //     ctx.body(fileBuffer),
  //   );
  // }),
  http.get("/api/v2/files/:fileId", () => {
    const fileBuffer = fs.readFileSync(
      path.resolve(__dirname, "./templateFiles.tar"),
    );
    return HttpResponse.json(fileBuffer, {
      headers: {
        "Content-Type": "application/octet-stream",
        "Content-Length": fileBuffer.byteLength.toString(),
      },
    });
  }),

  // http.get(
  //   "/api/v2/templateversions/:templateVersionId/parameters",
  //   (_, res, ctx) => {
  //     return res(
  //       ctx.status(200),
  //       ctx.json([
  //         M.MockTemplateVersionParameter1,
  //         M.MockTemplateVersionParameter2,
  //         M.MockTemplateVersionParameter3,
  //       ]),
  //     );
  //   },
  // ),
  http.get("/api/v2/templateversions/:templateVersionId/parameters", () => {
    return HttpResponse.json(
      [
        M.MockTemplateVersionParameter1,
        M.MockTemplateVersionParameter2,
        M.MockTemplateVersionParameter3,
      ],
      { status: 200 },
    );
  }),

  // http.get(
  //   "/api/v2/templateversions/:templateVersionId/variables",
  //   (_, res, ctx) => {
  //     return res(
  //       ctx.status(200),
  //       ctx.json([
  //         M.MockTemplateVersionVariable1,
  //         M.MockTemplateVersionVariable2,
  //         M.MockTemplateVersionVariable3,
  //       ]),
  //     );
  //   },
  // ),

  http.get("/api/v2/templateversions/:templateVersionId/variables", () => {
    return HttpResponse.json(
      [
        M.MockTemplateVersionVariable1,
        M.MockTemplateVersionVariable2,
        M.MockTemplateVersionVariable3,
      ],
      { status: 200 },
    );
  }),

  // http.get("/api/v2/deployment/ssh", (_, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockDeploymentSSH));
  // }),
  http.get("/api/v2/deployment/ssh", () => {
    return HttpResponse.json(M.MockDeploymentSSH, { status: 200 });
  }),

  // http.get("/api/v2/workspaceagents/:agent/logs", (_, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockWorkspaceAgentLogs));
  // }),
  http.get("/api/v2/workspaceagents/:agent/logs", () => {
    return HttpResponse.json(M.MockWorkspaceAgentLogs, { status: 200 });
  }),

  // http.get("/api/v2/debug/health", (_, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockHealth));
  // }),
  http.get("/api/v2/debug/health", () => {
    return HttpResponse.json(M.MockHealth, { status: 200 });
  }),

  // http.get("/api/v2/workspaceagents/:agent/listening-ports", (_, res, ctx) => {
  //   return res(ctx.status(200), ctx.json(M.MockListeningPortsResponse));
  // }),
  http.get("/api/v2/workspaceagents/:agent/listening-ports", () => {
    return HttpResponse.json(M.MockListeningPortsResponse, { status: 200 });
  }),
];
