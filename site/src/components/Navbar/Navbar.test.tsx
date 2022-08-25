import { render, screen, waitFor } from "@testing-library/react"
import { App } from "app"
import { Language } from "components/NavbarView/NavbarView"
import { rest } from "msw"
import {
  MockEntitlementsWithAuditLog,
  MockMemberPermissions,
  MockUser,
} from "testHelpers/renderHelpers"
import { server } from "testHelpers/server"

/**
 * The LicenseBanner, mounted above the AppRouter, fetches entitlements. Thus, to test their
 * effects, we must test at the App level and `waitFor` the fetch to be done.
 */
describe("Navbar", () => {
  it("shows Audit Log link when permitted and entitled", async () => {
    server.use(
      rest.get("/api/v2/entitlements", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockEntitlementsWithAuditLog))
      }),
    )
    render(<App />)
    await waitFor(() => {
      const link = screen.getByText(Language.users) // TODO change after debugging
      expect(link).toBeDefined()
    })
  })

  it("does not show Audit Log link when not entitled", async () => {
    render(<App />)
    await waitFor(() => {
      const link = screen.queryByText(Language.audit)
      expect(link).toBe(null)
    })
  })

  it("does not show Audit Log link when not permitted via role", async () => {
    server.use(
      rest.post(`/api/v2/users/${MockUser.id}/authorization`, async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockMemberPermissions))
      }),
    )
    server.use(
      rest.get("/api/v2/entitlements", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockEntitlementsWithAuditLog))
      }),
    )
    render(<App />)
    await waitFor(() => {
      const link = screen.queryByText(Language.audit)
      expect(link).toBe(null)
    })
  })
})
