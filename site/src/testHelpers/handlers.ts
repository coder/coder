import { rest } from "msw"
import { WorkspaceBuildTransition } from "../api/types"
import { CreateWorkspaceBuildRequest } from "../api/typesGenerated"
import { permissionsToCheck } from "../xServices/auth/authXService"
import * as M from "./entities"

export const handlers = [
  rest.get("/api/v2/metrics/daus", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockGetDAUResponse))
  }),

  // build info
  rest.get("/api/v2/buildinfo", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockBuildInfo))
  }),

  // organizations
  rest.get("/api/v2/organizations/:organizationId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockOrganization))
  }),
  rest.get("/api/v2/organizations/:organizationId/templates/:templateId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockTemplate))
  }),
  rest.get("/api/v2/organizations/:organizationId/templates", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([M.MockTemplate]))
  }),

  // templates
  rest.get("/api/v2/templates/:templateId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockTemplate))
  }),
  rest.get("/api/v2/templates/:templateId/versions", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([M.MockTemplateVersion]))
  }),
  rest.patch("/api/v2/templates/:templateId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockTemplate))
  }),
  rest.get("/api/v2/templateversions/:templateVersionId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockTemplateVersion))
  }),
  rest.get("/api/v2/templateversions/:templateVersionId/schema", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([]))
  }),
  rest.get("/api/v2/templateversions/:templateVersionId/resources", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([M.MockWorkspaceResource, M.MockWorkspaceResource2]))
  }),

  // users
  rest.get("/api/v2/users", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([M.MockUser, M.MockUser2, M.SuspendedMockUser]))
  }),
  rest.post("/api/v2/users", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockUser))
  }),
  rest.get("/api/v2/users/me/organizations", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([M.MockOrganization]))
  }),
  rest.get("/api/v2/users/me/organizations/:organizationId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockOrganization))
  }),
  rest.post("/api/v2/users/login", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockSessionToken))
  }),
  rest.post("/api/v2/users/logout", async (req, res, ctx) => {
    return res(ctx.status(200))
  }),
  rest.get("/api/v2/users/me", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockUser))
  }),
  rest.get("/api/v2/users/me/keys", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockAPIKey))
  }),
  rest.get("/api/v2/users/authmethods", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockAuthMethods))
  }),
  rest.get("/api/v2/users/roles", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockSiteRoles))
  }),
  rest.post("/api/v2/users/:userId/authorization", async (req, res, ctx) => {
    const permissions = Object.keys(permissionsToCheck)
    const response = permissions.reduce((obj, permission) => {
      return {
        ...obj,
        [permission]: true,
      }
    }, {})

    return res(ctx.status(200), ctx.json(response))
  }),
  rest.get("/api/v2/users/:userId/gitsshkey", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockGitSSHKey))
  }),
  rest.get("/api/v2/users/:userId/workspace/:workspaceName", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockWorkspace))
  }),

  // First user
  rest.get("/api/v2/users/first", async (req, res, ctx) => {
    return res(ctx.status(200))
  }),
  rest.post("/api/v2/users/first", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockUser))
  }),

  // workspaces
  rest.get("/api/v2/workspaces", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([M.MockWorkspace]))
  }),
  rest.get("/api/v2/workspaces/:workspaceId", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockWorkspace))
  }),
  rest.put("/api/v2/workspaces/:workspaceId/autostart", async (req, res, ctx) => {
    return res(ctx.status(200))
  }),
  rest.put("/api/v2/workspaces/:workspaceId/ttl", async (req, res, ctx) => {
    return res(ctx.status(200))
  }),
  rest.put("/api/v2/workspaces/:workspaceId/extend", async (req, res, ctx) => {
    return res(ctx.status(200))
  }),

  // workspace builds
  rest.post("/api/v2/workspaces/:workspaceId/builds", async (req, res, ctx) => {
    const { transition } = req.body as CreateWorkspaceBuildRequest
    const transitionToBuild = {
      start: M.MockWorkspaceBuild,
      stop: M.MockWorkspaceBuildStop,
      delete: M.MockWorkspaceBuildDelete,
    }
    const result = transitionToBuild[transition as WorkspaceBuildTransition]
    return res(ctx.status(200), ctx.json(result))
  }),
  rest.get("/api/v2/workspaces/:workspaceId/builds", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockBuilds))
  }),
  rest.get(
    "/api/v2/users/:username/workspace/:workspaceName/builds/:buildNumber",
    (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(M.MockWorkspaceBuild))
    },
  ),
  rest.get("/api/v2/workspacebuilds/:workspaceBuildId/resources", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json([M.MockWorkspaceResource, M.MockWorkspaceResource2]))
  }),
  rest.patch("/api/v2/workspacebuilds/:workspaceBuildId/cancel", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockCancellationMessage))
  }),
  rest.get("/api/v2/workspacebuilds/:workspaceBuildId/logs", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockWorkspaceBuildLogs))
  }),
  rest.get("/api/v2/entitlements", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockEntitlements))
  }),
]
