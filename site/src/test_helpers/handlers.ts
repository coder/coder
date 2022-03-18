import { rest } from "msw"
import * as M from "./entities"

export const handlers = [
  rest.post("/api/v2/users/me/workspaces", async (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockWorkspace))
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
]
