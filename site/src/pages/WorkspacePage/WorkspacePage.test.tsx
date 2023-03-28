import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import EventSourceMock from "eventsourcemock"
import i18next from "i18next"
import { rest } from "msw"
import * as api from "../../api/api"
import { Workspace } from "../../api/typesGenerated"
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
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockWorkspace,
  MockWorkspaceBuild,
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "../../testHelpers/renderHelpers"
import { server } from "../../testHelpers/server"
import { WorkspacePage } from "./WorkspacePage"

const { t } = i18next

// It renders the workspace page and waits for it be loaded
const renderWorkspacePage = async () => {
  jest.spyOn(api, "getTemplate").mockResolvedValueOnce(MockTemplate)
  jest.spyOn(api, "getTemplateVersionRichParameters").mockResolvedValueOnce([])
  renderWithAuth(<WorkspacePage />, {
    route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}`,
    path: "/@:username/:workspace",
  })

  await waitForLoaderToBeRemoved()
}

/**
 * Requests and responses related to workspace status are unrelated, so we can't test in the usual way.
 * Instead, test that button clicks produce the correct requests and that responses produce the correct UI.
 * We don't need to test the UI exhaustively because Storybook does that; just enough to prove that the
 * workspaceStatus was calculated correctly.
 */
const testButton = async (label: string, actionMock: jest.SpyInstance) => {
  const user = userEvent.setup()
  await renderWorkspacePage()
  const workspaceActions = screen.getByTestId("workspace-actions")
  const button = within(workspaceActions).getByRole("button", { name: label })
  await user.click(button)
  expect(actionMock).toBeCalled()
}

const testStatus = async (ws: Workspace, label: string) => {
  server.use(
    rest.get(
      `/api/v2/users/:username/workspace/:workspaceName`,
      (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(ws))
      },
    ),
  )
  await renderWorkspacePage()
  const header = screen.getByTestId("header")
  const status = within(header).getByRole("status")
  expect(status).toHaveTextContent(label)
}

let originalEventSource: typeof window.EventSource

beforeAll(() => {
  originalEventSource = window.EventSource
  // mocking out EventSource for SSE
  window.EventSource = EventSourceMock
})

beforeEach(() => {
  jest.resetAllMocks()
})

afterAll(() => {
  window.EventSource = originalEventSource
})

describe("WorkspacePage", () => {
  it("requests a delete job when the user presses Delete and confirms", async () => {
    const user = userEvent.setup({ delay: 0 })
    const deleteWorkspaceMock = jest
      .spyOn(api, "deleteWorkspace")
      .mockResolvedValueOnce(MockWorkspaceBuild)
    await renderWorkspacePage()

    // open the workspace action popover so we have access to all available ctas
    const trigger = screen.getByTestId("workspace-actions-button")
    await user.click(trigger)
    const buttonText = t("actionButton.delete", { ns: "workspacePage" })

    // Click on delete
    const button = await screen.findByText(buttonText)
    await user.click(button)

    // Get dialog and confirm
    const dialog = await screen.findByTestId("dialog")
    const labelText = t("deleteDialog.confirmLabel", {
      ns: "common",
      entity: "workspace",
    })
    const textField = within(dialog).getByLabelText(labelText)
    await user.type(textField, MockWorkspace.name)
    const confirmButton = within(dialog).getByRole("button", {
      name: "Delete",
      hidden: false,
    })
    await user.click(confirmButton)
    expect(deleteWorkspaceMock).toBeCalled()
  })

  it("requests a start job when the user presses Start", async () => {
    server.use(
      rest.get(
        `/api/v2/users/:userId/workspace/:workspaceName`,
        (req, res, ctx) => {
          return res(ctx.status(200), ctx.json(MockStoppedWorkspace))
        },
      ),
    )
    const startWorkspaceMock = jest
      .spyOn(api, "startWorkspace")
      .mockImplementation(() => Promise.resolve(MockWorkspaceBuild))
    await testButton(
      t("actionButton.start", { ns: "workspacePage" }),
      startWorkspaceMock,
    )
  })

  it("requests a stop job when the user presses Stop", async () => {
    const stopWorkspaceMock = jest
      .spyOn(api, "stopWorkspace")
      .mockResolvedValueOnce(MockWorkspaceBuild)

    await testButton(
      t("actionButton.stop", { ns: "workspacePage" }),
      stopWorkspaceMock,
    )
  })

  it("requests cancellation when the user presses Cancel", async () => {
    server.use(
      rest.get(
        `/api/v2/users/:userId/workspace/:workspaceName`,
        (req, res, ctx) => {
          return res(ctx.status(200), ctx.json(MockStartingWorkspace))
        },
      ),
    )
    const cancelWorkspaceMock = jest
      .spyOn(api, "cancelWorkspaceBuild")
      .mockImplementation(() => Promise.resolve({ message: "job canceled" }))

    await renderWorkspacePage()

    const workspaceActions = screen.getByTestId("workspace-actions")
    const cancelButton = within(workspaceActions).getByRole("button", {
      name: "cancel action",
    })

    await userEvent.setup().click(cancelButton)

    expect(cancelWorkspaceMock).toBeCalled()
  })

  it("requests an update when the user presses Update", async () => {
    jest
      .spyOn(api, "getWorkspaceByOwnerAndName")
      .mockResolvedValueOnce(MockOutdatedWorkspace)
    const updateWorkspaceMock = jest
      .spyOn(api, "updateWorkspace")
      .mockResolvedValueOnce(MockWorkspaceBuild)

    await testButton(
      t("actionButton.update", { ns: "workspacePage" }),
      updateWorkspaceMock,
    )
  })

  it("updates the parameters when they are missing during update", async () => {
    // Setup mocks
    const user = userEvent.setup()
    jest
      .spyOn(api, "getWorkspaceByOwnerAndName")
      .mockResolvedValueOnce(MockOutdatedWorkspace)
    const updateWorkspaceSpy = jest
      .spyOn(api, "updateWorkspace")
      .mockRejectedValueOnce(
        new api.MissingBuildParameters([
          MockTemplateVersionParameter1,
          MockTemplateVersionParameter2,
        ]),
      )
    // Render page and wait for it to be loaded
    renderWithAuth(<WorkspacePage />, {
      route: `/@${MockWorkspace.owner_name}/${MockWorkspace.name}`,
      path: "/@:username/:workspace",
    })
    await waitForLoaderToBeRemoved()
    // Click on the update button
    const workspaceActions = screen.getByTestId("workspace-actions")
    await user.click(
      within(workspaceActions).getByRole("button", { name: "Update" }),
    )
    await waitFor(() => {
      expect(api.updateWorkspace).toBeCalled()
      // We want to clear this mock to use it later
      updateWorkspaceSpy.mockClear()
    })
    // Fill the parameters and send the form
    const dialog = await screen.findByTestId("dialog")
    const firstParameterInput = within(dialog).getByLabelText(
      MockTemplateVersionParameter1.name,
      { exact: false },
    )
    await user.clear(firstParameterInput)
    await user.type(firstParameterInput, "some-value")
    const secondParameterInput = within(dialog).getByLabelText(
      MockTemplateVersionParameter2.name,
      { exact: false },
    )
    await user.clear(secondParameterInput)
    await user.type(secondParameterInput, "2")
    await user.click(within(dialog).getByRole("button", { name: "Update" }))
    // Check if the update was called using the values from the form
    await waitFor(() => {
      expect(api.updateWorkspace).toBeCalledWith(MockOutdatedWorkspace, [
        {
          name: MockTemplateVersionParameter1.name,
          value: "some-value",
        },
        {
          name: MockTemplateVersionParameter2.name,
          value: "2",
        },
      ])
    })
  })

  it("shows the Stopping status when the workspace is stopping", async () => {
    await testStatus(
      MockStoppingWorkspace,
      t("workspaceStatus.stopping", { ns: "common" }),
    )
  })

  it("shows the Stopped status when the workspace is stopped", async () => {
    await testStatus(
      MockStoppedWorkspace,
      t("workspaceStatus.stopped", { ns: "common" }),
    )
  })

  it("shows the Building status when the workspace is starting", async () => {
    await testStatus(
      MockStartingWorkspace,
      t("workspaceStatus.starting", { ns: "common" }),
    )
  })

  it("shows the Running status when the workspace is running", async () => {
    await testStatus(
      MockWorkspace,
      t("workspaceStatus.running", { ns: "common" }),
    )
  })

  it("shows the Failed status when the workspace is failed or canceled", async () => {
    await testStatus(
      MockFailedWorkspace,
      t("workspaceStatus.failed", { ns: "common" }),
    )
  })

  it("shows the Canceling status when the workspace is canceling", async () => {
    await testStatus(
      MockCancelingWorkspace,
      t("workspaceStatus.canceling", { ns: "common" }),
    )
  })

  it("shows the Canceled status when the workspace is canceling", async () => {
    await testStatus(
      MockCanceledWorkspace,
      t("workspaceStatus.canceled", { ns: "common" }),
    )
  })

  it("shows the Deleting status when the workspace is deleting", async () => {
    await testStatus(
      MockDeletingWorkspace,
      t("workspaceStatus.deleting", { ns: "common" }),
    )
  })

  it("shows the Deleted status when the workspace is deleted", async () => {
    await testStatus(
      MockDeletedWorkspace,
      t("workspaceStatus.deleted", { ns: "common" }),
    )
  })

  it("shows the timeline build", async () => {
    await renderWorkspacePage()
    const table = await screen.findByTestId("builds-table")

    // Wait for the results to be loaded
    await waitFor(async () => {
      const rows = table.querySelectorAll("tbody > tr")
      // Added +1 because of the date row
      expect(rows).toHaveLength(MockBuilds.length + 1)
    })
  })
})
