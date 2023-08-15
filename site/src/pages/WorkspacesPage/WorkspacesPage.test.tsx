import { screen, waitFor, within } from "@testing-library/react"
import { rest } from "msw"
import * as CreateDayString from "utils/createDayString"
import { MockWorkspace, MockWorkspacesResponse } from "testHelpers/entities"
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers"
import { server } from "testHelpers/server"
import WorkspacesPage from "./WorkspacesPage"
import { i18n } from "i18n"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { Workspace } from "api/typesGenerated"

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

  it("deletes only the selected workspaces", async () => {
    const workspaces = [
      { ...MockWorkspace, id: "1" },
      { ...MockWorkspace, id: "2" },
      { ...MockWorkspace, id: "3" },
    ]
    jest
      .spyOn(API, "getWorkspaces")
      .mockResolvedValue({ workspaces, count: workspaces.length })
    const deleteWorkspace = jest.spyOn(API, "deleteWorkspace")
    const user = userEvent.setup()
    renderWithAuth(<WorkspacesPage />)
    await waitForLoaderToBeRemoved()

    await user.click(getWorkspaceCheckbox(workspaces[0]))
    await user.click(getWorkspaceCheckbox(workspaces[1]))
    await user.click(screen.getByRole("button", { name: /delete all/i }))
    await user.type(screen.getByLabelText(/type delete to confirm/i), "DELETE")
    await user.click(screen.getByTestId("confirm-button"))

    await waitFor(() => {
      expect(deleteWorkspace).toHaveBeenCalledTimes(2)
    })
    expect(deleteWorkspace).toHaveBeenCalledWith(workspaces[0].id)
    expect(deleteWorkspace).toHaveBeenCalledWith(workspaces[1].id)
  })
})

const getWorkspaceCheckbox = (workspace: Workspace) => {
  return within(screen.getByTestId(`checkbox-${workspace.id}`)).getByRole(
    "checkbox",
  )
}
