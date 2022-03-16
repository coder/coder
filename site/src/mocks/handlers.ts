import { rest } from 'msw'
import * as M from './entities'

export const handlers = [
  rest.post("/api/v2/users/me/workspaces", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockWorkspace))
  }),
  rest.post("api/v2/users/login", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockSessionToken))
  }),
  rest.post("/api/v2/users/logout", (req, res, ctx) => {
    return res(ctx.status(200))
  }),
  rest.get("api/v2/users/me", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockUser))
  }),
  rest.get("api/v2/users/me/keys", (req, res, ctx) => {
    return res(ctx.status(200), ctx.json(M.MockAPIKey))
  })
]