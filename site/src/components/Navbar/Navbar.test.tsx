import { render, MockEntitlementsWithAuditLog, MockMemberPermissions } from "testHelpers/renderHelpers"
import { server } from "testHelpers/server"
import { screen } from "@testing-library/react"
import { Navbar } from "./Navbar"
import { rest } from "msw"

describe("Navbar", () => {
  it("shows Audit Log link when permitted and entitled", () => {
    server.use(
      rest.get("/api/entitlements", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockEntitlementsWithAuditLog))
      }),
    )
    render(<Navbar />)
    expect(screen.getByText("Audit Log"))
  })

  it("does not show Audit Log link when not entitled", () => {
    server.use()
    render(<Navbar />)
    expect(screen.queryByText("Audit Log")).not.toBeDefined()
  })

  it("does not show Audit Log link when not permitted via role", () => {
     server.use(
      rest.post("/api/v2/users/:userId/authorization", async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockMemberPermissions))
      }),
    )
    render(<Navbar />)
    expect(screen.queryByText("Audit Log")).not.toBeDefined()
  })
})
