import { screen } from "@testing-library/react"
import { rest } from "msw"
import { MockEntitlementsWithWarnings } from "testHelpers/entities"
import { render } from "testHelpers/renderHelpers"
import { server } from "testHelpers/server"
import { LicenseBanner } from "./LicenseBanner"
import { Language } from "./LicenseBannerView"

describe("LicenseBanner", () => {
  it("does not show when there are no warnings", async () => {
    render(<LicenseBanner />)
    const bannerPillSingular = await screen.queryByText(Language.licenseIssue)
    const bannerPillPlural = await screen.queryByText(Language.licenseIssues(2))
    expect(bannerPillSingular).toBe(null)
    expect(bannerPillPlural).toBe(null)
  })
  it("shows when there are warnings", async () => {
    server.use(
      rest.get("/api/v2/entitlements", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockEntitlementsWithWarnings))
      }),
    )
    render(<LicenseBanner />)
    const bannerPill = await screen.findByText(Language.licenseIssues(2))
    expect(bannerPill).toBeDefined()
  })
})
