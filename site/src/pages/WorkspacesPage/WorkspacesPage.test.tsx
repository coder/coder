import { screen } from "@testing-library/react"
import { rest } from "msw"
import * as CreateDayString from "utils/createDayString"
import { MockWorkspace, MockWorkspacesResponse } from "testHelpers/entities"
import { renderWithAuth } from "testHelpers/renderHelpers"
import { server } from "testHelpers/server"
import WorkspacesPage from "./WorkspacesPage"
import { i18n } from "i18n"

const { t } = i18n

describe("WorkspacesPage", () => {
  beforeEach(() => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString")
    mock.mockImplementation(() => "a minute ago")
  })

  it("renders an empty workspaces page", async () => {
    // Given
    server.use(
      rest.get("/api/v2/workspaces", async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json({ workspaces: [], count: 0 }))
      }),
    )

    // When
    renderWithAuth(<WorkspacesPage />)

    // Then
    const text = t("emptyCreateWorkspaceMessage", { ns: "workspacesPage" })
    await screen.findByText(text)
  })

  it("renders a filled workspaces page", async () => {
    renderWithAuth(<WorkspacesPage />)
    await screen.findByText(`${MockWorkspace.name}1`)
    const templateDisplayNames = await screen.findAllByText(
      `${MockWorkspace.template_display_name}`,
    )
    expect(templateDisplayNames).toHaveLength(MockWorkspacesResponse.count)
  })
})
