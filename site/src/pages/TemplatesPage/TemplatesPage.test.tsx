import { screen } from "@testing-library/react"
import { rest } from "msw"
import * as CreateDayString from "util/createDayString"
import { MockTemplate } from "../../testHelpers/entities"
import { history, render } from "../../testHelpers/renderHelpers"
import { server } from "../../testHelpers/server"
import { TemplatesPage } from "./TemplatesPage"
import { Language } from "./TemplatesPageView"

describe("TemplatesPage", () => {
  beforeEach(() => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString")
    mock.mockImplementation(() => "a minute ago")
    history.replace("/workspaces")
  })

  it("renders an empty templates page", async () => {
    // Given
    server.use(
      rest.get(
        "/api/v2/organizations/:organizationId/templates",
        (req, res, ctx) => {
          return res(ctx.status(200), ctx.json([]))
        },
      ),
      rest.post("/api/v2/authcheck", (req, res, ctx) => {
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
    await screen.findByText(Language.emptyMessage)
  })

  it("renders a filled templates page", async () => {
    // When
    render(<TemplatesPage />)

    // Then
    await screen.findByText(MockTemplate.display_name)
  })

  it("shows empty view without permissions to create", async () => {
    server.use(
      rest.get(
        "/api/v2/organizations/:organizationId/templates",
        (req, res, ctx) => {
          return res(ctx.status(200), ctx.json([]))
        },
      ),
      rest.post("/api/v2/authcheck", (req, res, ctx) => {
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
