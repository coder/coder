import { screen } from "@testing-library/react"
import { rest } from "msw"
import {
  MockEntitlementsWithAuditLog,
  MockMemberPermissions,
  MockUser,
  render,
} from "testHelpers/renderHelpers"
import { server } from "testHelpers/server"
import { Navbar } from "./Navbar"

describe("Navbar", () => {
  it("shows Audit Log link when permitted and entitled", async () => {
    server.use(
      rest.get("/api/entitlements", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockEntitlementsWithAuditLog))
      }),
    )
    render(<Navbar />)
    const link = await screen.findByText("Audit")
    expect(link).toBeDefined()
  })

  it("does not show Audit Log link when not entitled", () => {
    render(<Navbar />)
    const link = screen.getByText("Audit")
    expect(link).not.toBeDefined()
  })

  it("does not show Audit Log link when not permitted via role", () => {
    server.use(
      rest.post(`/api/v2/users/${MockUser.id}/authorization`, async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockMemberPermissions))
      }),
    )
    render(<Navbar />)
    const link = screen.getByText("Audit")
    expect(link).not.toBeDefined()
  })
})
