/* eslint-disable @typescript-eslint/no-floating-promises */
import { screen } from "@testing-library/react"
import { rest } from "msw"
import React from "react"
import * as api from "../../api/api"
import { Template, Workspace, WorkspaceBuild } from "../../api/typesGenerated"
import { Language } from "../../components/WorkspaceStatusBar/WorkspaceStatusBar"
import {
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
  MockWorkspaceBuild,
  renderWithAuth,
} from "../../testHelpers/renderHelpers"
import { server } from "../../testHelpers/server"
import { WorkspacePage } from "./WorkspacePage"

/**
 * Requests and responses related to workspace status are unrelated, so we can't test in the usual way.
 * Instead, test that button clicks produce the correct requests and that responses produce the correct UI.
 * We don't need to test the UI exhaustively because Storybook does that; just enough to prove that the
 * workspaceStatus was calculated correctly.
 */

const testButton = async (
  label: string,
  mock:
    | jest.SpyInstance<Promise<WorkspaceBuild>, [workspaceId: string, templateVersionId?: string | undefined]>
    | jest.SpyInstance<Promise<Template>, [templateId: string]>,
) => {
  renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
  const button = await screen.findByText(label)
  button.click()
  expect(mock).toHaveBeenCalled()
}

const testStatus = async (mock: Workspace, label: string) => {
  server.use(
    rest.get(`/api/v2/workspaces/${MockWorkspace.id}`, (req, res, ctx) => {
      return res(ctx.status(200), ctx.json(mock))
    }),
  )
  renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
  const status = await screen.findByRole("status")
  expect(status).toHaveTextContent(label)
}

describe("Workspace Page", () => {
  it("shows a workspace", async () => {
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
    const workspaceName = await screen.findByText(MockWorkspace.name)
    expect(workspaceName).toBeDefined()
  })
  it("shows the status of the workspace", async () => {
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
    const status = await screen.findByRole("status")
    expect(status).toHaveTextContent("Running")
  })
  it("requests a stop job when the user presses Stop", async () => {
    const stopWorkspaceMock = jest
      .spyOn(api, "stopWorkspace")
      .mockImplementation(() => Promise.resolve(MockWorkspaceBuild))
    testButton(Language.start, stopWorkspaceMock)
  }),
    it("requests a start job when the user presses Start", async () => {
      const startWorkspaceMock = jest
        .spyOn(api, "startWorkspace")
        .mockImplementation(() => Promise.resolve(MockWorkspaceBuild))
      testButton(Language.start, startWorkspaceMock)
    }),
    it("requests a start job when the user presses Retry after trying to start", async () => {
      const startWorkspaceMock = jest
        .spyOn(api, "startWorkspace")
        .mockImplementation(() => Promise.resolve(MockWorkspaceBuild))
      testButton(Language.retry, startWorkspaceMock)
    }),
    it("requests a stop job when the user presses Retry after trying to stop", async () => {
      const stopWorkspaceMock = jest
        .spyOn(api, "stopWorkspace")
        .mockImplementation(() => Promise.resolve(MockWorkspaceBuild))
      server.use(
        rest.get(`/api/v2/workspaces/${MockWorkspace.id}`, (req, res, ctx) => {
          return res(ctx.status(200), ctx.json(MockStoppedWorkspace))
        }),
      )
      testButton(Language.start, stopWorkspaceMock)
    }),
    it("requests a template when the user presses Update", async () => {
      const getTemplateMock = jest.spyOn(api, "getTemplate").mockImplementation(() => Promise.resolve(MockTemplate))
      server.use(
        rest.get(`/api/v2/workspaces/${MockWorkspace.id}`, (req, res, ctx) => {
          return res(ctx.status(200), ctx.json(MockOutdatedWorkspace))
        }),
      )
      testButton(Language.update, getTemplateMock)
    }),
    it("shows the Stopping status when the workspace is stopping", async () => {
      testStatus(MockStoppingWorkspace, Language.stopping)
    })
  it("shows the Stopped status when the workspace is stopped", async () => {
    testStatus(MockStoppedWorkspace, Language.stopped)
  })
  it("shows the Building status when the workspace is starting", async () => {
    testStatus(MockStartingWorkspace, Language.starting)
  })
  it("shows the Running status when the workspace is started", async () => {
    testStatus(MockWorkspace, Language.started)
  })
  it("shows the Error status when the workspace is failed or canceled", async () => {
    testStatus(MockFailedWorkspace, Language.error)
  })
  it("shows the Loading status when the workspace is canceling", async () => {
    testStatus(MockCancelingWorkspace, Language.canceling)
  })
  it("shows the Deleting status when the workspace is deleting", async () => {
    testStatus(MockDeletingWorkspace, Language.canceling)
  })
  it("shows the Deleted status when the workspace is deleted", async () => {
    testStatus(MockDeletedWorkspace, Language.canceling)
  })
})
