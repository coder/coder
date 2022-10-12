import { screen } from "@testing-library/react"
import { rest } from "msw"
import * as CreateDayString from "util/createDayString"
import { Language as WorkspacesTableBodyLanguage } from "../../components/WorkspacesTable/WorkspacesTableBody"
import { MockWorkspace } from "../../testHelpers/entities"
import { history, render } from "../../testHelpers/renderHelpers"
import { server } from "../../testHelpers/server"
import WorkspacesPage from "./WorkspacesPage"

describe("WorkspacesPage", () => {
  beforeEach(() => {
    history.replace("/workspaces")
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString")
    mock.mockImplementation(() => "a minute ago")
  })

  it("renders an empty workspaces page", async () => {
    // Given
    server.use(
      rest.get("/api/v2/workspaces", async (req, res, ctx) => {
        return res(ctx.status(200), ctx.json([]))
      }),
    )

    // When
    render(<WorkspacesPage />)

    // Then
    await screen.findByText(
      WorkspacesTableBodyLanguage.emptyCreateWorkspaceMessage,
    )
  })

  it("renders a filled workspaces page", async () => {
    // When
    render(<WorkspacesPage />)

    // Then
    await screen.findByText(MockWorkspace.name)
  })
})
