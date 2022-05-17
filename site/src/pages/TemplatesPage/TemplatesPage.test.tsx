import { screen } from "@testing-library/react"
import { rest } from "msw"
import React from "react"
import { MockTemplate } from "../../testHelpers/entities"
import { history, render } from "../../testHelpers/renderHelpers"
import { server } from "../../testHelpers/server"
import TemplatesPage from "./TemplatesPage"
import { Language } from "./TemplatesPageView"

describe("TemplatesPage", () => {
  beforeEach(() => {
    history.replace("/workspaces")
  })

  it("renders an empty templates page", async () => {
    // Given
    server.use(
      rest.get("/api/v2/organizations/:organizationId/templates", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json([]))
      }),
      rest.post("/api/v2/users/:userId/authorization", async (req, res, ctx) => {
        return res(
          ctx.status(200),
          ctx.json({
            createTemplates: true,
          }),
        )
      }),
    )

    // When
    render(<TemplatesPage />)

    // Then
    await screen.findByText(Language.emptyViewCreate)
  })

  it("renders a filled templates page", async () => {
    // When
    render(<TemplatesPage />)

    // Then
    await screen.findByText(MockTemplate.name)
  })

  it("shows empty view without permissions to create", async () => {
    server.use(
      rest.get("/api/v2/organizations/:organizationId/templates", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json([]))
      }),
      rest.post("/api/v2/users/:userId/authorization", async (req, res, ctx) => {
        return res(
          ctx.status(200),
          ctx.json({
            createTemplates: false,
          }),
        )
      }),
    )

    // When
    render(<TemplatesPage />)

    // Then
    await screen.findByText(Language.emptyViewNoPerms)
  })
})
