import { screen } from "@testing-library/react"
import { rest } from "msw"
import * as CreateDayString from "utils/createDayString"
import {
  MockWorkspace,
  MockWorkspacesResponse,
  MockEntitlementsWithScheduling,
  MockWorkspacesResponseWithDeletions,
} from "testHelpers/entities"
import { history, renderWithAuth } from "testHelpers/renderHelpers"
import { server } from "testHelpers/server"
import WorkspacesPage from "./WorkspacesPage"
import { i18n } from "i18n"
import * as API from "api/api"
import userEvent from "@testing-library/user-event"

const { t } = i18n

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

  it("displays banner for impending deletions", async () => {
    jest
      .spyOn(API, "getEntitlements")
      .mockResolvedValue(MockEntitlementsWithScheduling)

    jest
      .spyOn(API, "getWorkspaces")
      .mockResolvedValue(MockWorkspacesResponseWithDeletions)

    renderWithAuth(<WorkspacesPage />)

    const banner = await screen.findByText(
      "You have workspaces that will be deleted soon due to inactivity. To keep these workspaces, connect to them via SSH or the web terminal.",
    )
    const user = userEvent.setup()
    await user.click(screen.getByTestId("dismiss-banner-btn"))

    expect(banner).toBeEmptyDOMElement
  })
})
