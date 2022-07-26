import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import { rest } from "msw"
import * as api from "../../api/api"
import { Workspace } from "../../api/typesGenerated"
import { Language } from "../../components/WorkspaceActions/ActionCtas"
import {
  MockBuilds,
  MockCanceledWorkspace,
  MockCancelingWorkspace,
  MockDeletedWorkspace,
  MockDeletingWorkspace,
  MockFailedWorkspace,
  MockOutdatedWorkspace,
  MockStartingWorkspace,
  MockStoppedWorkspace,
  MockStoppingWorkspace,
  MockTemplate,
  MockWorkspace,
  MockWorkspaceAgent,
  MockWorkspaceAgentDisconnected,
  MockWorkspaceBuild,
  renderWithAuth,
} from "../../testHelpers/renderHelpers"
import { server } from "../../testHelpers/server"
import { DisplayAgentStatusLanguage, DisplayStatusLanguage } from "../../util/workspace"
import { WorkspacePage } from "./WorkspacePage"

// It renders the workspace page and waits for it be loaded
const renderWorkspacePage = async () => {
  renderWithAuth(<WorkspacePage />, {
    route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}`,
    path: "/@:username/:workspace",
  })
  await screen.findByText(MockWorkspace.name)
}

/**
 * Requests and responses related to workspace status are unrelated, so we can't test in the usual way.
 * Instead, test that button clicks produce the correct requests and that responses produce the correct UI.
 * We don't need to test the UI exhaustively because Storybook does that; just enough to prove that the
 * workspaceStatus was calculated correctly.
 */

const testButton = async (label: string, actionMock: jest.SpyInstance) => {
  await renderWorkspacePage()
  // REMARK: exact here because the "Start" button and "START" label for
  //         workspace schedule could otherwise conflict.
  const button = await screen.findByText(label, { exact: true })
  await waitFor(() => fireEvent.click(button))
  expect(actionMock).toBeCalled()
}

const testStatus = async (mock: Workspace, label: string) => {
  server.use(
    rest.get(`/api/v2/users/:username/workspace/:workspaceName`, (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(mock))
    }),
  )
  await renderWorkspacePage()
  const status = await screen.findByRole("status")
  expect(status).toHaveTextContent(label)
}

beforeEach(() => {
  jest.resetAllMocks()
})

describe("Workspace Page", () => {
  it("shows a workspace", async () => {
    await renderWorkspacePage()
    const workspaceName = screen.getByText(MockWorkspace.name)
    expect(workspaceName).toBeDefined()
  })
  it("shows the status of the workspace", async () => {
    await renderWorkspacePage()
    const status = screen.getByRole("status")
    expect(status).toHaveTextContent("Running")
  })
  it("requests a stop job when the user presses Stop", async () => {
    const stopWorkspaceMock = jest
      .spyOn(api, "stopWorkspace")
      .mockResolvedValueOnce(MockWorkspaceBuild)
    await testButton(Language.stop, stopWorkspaceMock)
  })
  it("requests a delete job when the user presses Delete and confirms", async () => {
    const deleteWorkspaceMock = jest
      .spyOn(api, "deleteWorkspace")
      .mockResolvedValueOnce(MockWorkspaceBuild)
    await renderWorkspacePage()

    // open the workspace action popover so we have access to all available ctas
    const trigger = await screen.findByTestId("workspace-actions-button")
    trigger.click()

    const button = await screen.findByText(Language.delete)
    await waitFor(() => fireEvent.click(button))
    const confirmDialog = await screen.findByRole("dialog")
    const confirmButton = within(confirmDialog).getByText("Delete")
    await waitFor(() => fireEvent.click(confirmButton))
    expect(deleteWorkspaceMock).toBeCalled()
  })
  it("requests a start job when the user presses Start", async () => {
    server.use(
      rest.get(`/api/v2/users/:userId/workspace/:workspaceName`, (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockStoppedWorkspace))
      }),
    )
    const startWorkspaceMock = jest
      .spyOn(api, "startWorkspace")
      .mockImplementation(() => Promise.resolve(MockWorkspaceBuild))
    await testButton(Language.start, startWorkspaceMock)
  })
  it("requests cancellation when the user presses Cancel", async () => {
    server.use(
      rest.get(`/api/v2/users/:userId/workspace/:workspaceName`, (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockStartingWorkspace))
      }),
    )
    const cancelWorkspaceMock = jest
      .spyOn(api, "cancelWorkspaceBuild")
      .mockImplementation(() => Promise.resolve({ message: "job canceled" }))

    await renderWorkspacePage()

    const cancelButton = await screen.findByRole("button", {
      name: "cancel action",
    })
    await waitFor(() => fireEvent.click(cancelButton))

    expect(cancelWorkspaceMock).toBeCalled()
  })
  it("requests a template when the user presses Update", async () => {
    const getTemplateMock = jest.spyOn(api, "getTemplate").mockResolvedValueOnce(MockTemplate)
    server.use(
      rest.get(`/api/v2/users/:userId/workspace/:workspaceName`, (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockOutdatedWorkspace))
      }),
    )
    await testButton(Language.update, getTemplateMock)
  })
  it("shows the Stopping status when the workspace is stopping", async () => {
    await testStatus(MockStoppingWorkspace, DisplayStatusLanguage.stopping)
  })
  it("shows the Stopped status when the workspace is stopped", async () => {
    await testStatus(MockStoppedWorkspace, DisplayStatusLanguage.stopped)
  })
  it("shows the Building status when the workspace is starting", async () => {
    await testStatus(MockStartingWorkspace, DisplayStatusLanguage.starting)
  })
  it("shows the Running status when the workspace is started", async () => {
    await testStatus(MockWorkspace, DisplayStatusLanguage.started)
  })
  it("shows the Failed status when the workspace is failed or canceled", async () => {
    await testStatus(MockFailedWorkspace, DisplayStatusLanguage.failed)
  })
  it("shows the Canceling status when the workspace is canceling", async () => {
    await testStatus(MockCancelingWorkspace, DisplayStatusLanguage.canceling)
  })
  it("shows the Canceled status when the workspace is canceling", async () => {
    await testStatus(MockCanceledWorkspace, DisplayStatusLanguage.canceled)
  })
  it("shows the Deleting status when the workspace is deleting", async () => {
    await testStatus(MockDeletingWorkspace, DisplayStatusLanguage.deleting)
  })
  it("shows the Deleted status when the workspace is deleted", async () => {
    await testStatus(MockDeletedWorkspace, DisplayStatusLanguage.deleted)
  })

  describe("Timeline", () => {
    it("shows the timeline build", async () => {
      await renderWorkspacePage()
      const table = await screen.findByTestId("builds-table")

      // Wait for the results to be loaded
      await waitFor(async () => {
        const rows = table.querySelectorAll("tbody > tr")
        expect(rows).toHaveLength(MockBuilds.length)
      })
    })
  })

  describe("Resources", () => {
    it("shows the status of each agent in each resource", async () => {
      renderWithAuth(<WorkspacePage />, {
        route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}`,
        path: "/@:username/:workspace",
      })
      const agent1Names = await screen.findAllByText(MockWorkspaceAgent.name)
      expect(agent1Names.length).toEqual(2)
      const agent2Names = await screen.findAllByText(MockWorkspaceAgentDisconnected.name)
      expect(agent2Names.length).toEqual(2)
      const agent1Status = await screen.findAllByText(
        DisplayAgentStatusLanguage[MockWorkspaceAgent.status],
      )
      expect(agent1Status.length).toEqual(2)
      const agent2Status = await screen.findAllByText(
        DisplayAgentStatusLanguage[MockWorkspaceAgentDisconnected.status],
      )
      expect(agent2Status.length).toEqual(2)
    })
  })
})
