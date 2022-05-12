import { screen } from "@testing-library/react"
import { rest } from "msw"
import React from "react"
import {
  MockFailedWorkspace,
  MockOutdatedWorkspace,
  MockStoppedWorkspace,
  MockTemplate,
  MockWorkspace,
  renderWithAuth,
} from "../../testHelpers"
import { server } from "../../testHelpers/server"
import { WorkspacePage } from "./WorkspacePage"

describe("Workspace Page", () => {
  it("shows a workspace", async () => {
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
    const workspaceName = await screen.findByText(MockWorkspace.name)
    const templateName = await screen.findByText(MockTemplate.name)
    expect(workspaceName).toBeDefined()
    expect(templateName).toBeDefined()
  })
  it("shows the status of the workspace", async () => {
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
    const status = await screen.findByRole("status")
    expect(status).toHaveTextContent("Running")
  })
  it("stops the workspace when the user presses Stop", async () => {
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
    const status = await screen.findByText("Running")
    expect(status).toBeDefined()
    const stopButton = await screen.findByText("Stop")
    stopButton.click()
    const laterStatus = await screen.findByText("Stopping")
    expect(laterStatus).toBeDefined()
  })
  it("starts the workspace when the user presses Start", async () => {
    server.use(
      rest.get(`/api/v2/workspaces/${MockWorkspace.id}`, (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockStoppedWorkspace))
      }),
    )
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
    const startButton = await screen.findByText("Start")
    const status = await screen.findByText("Stopped")
    expect(status).toBeDefined()
    startButton.click()
    const laterStatus = await screen.findByText("Building")
    expect(laterStatus).toBeDefined()
  })
  it("retries starting the workspace when the user presses Retry", async () => {
    // MockFailedWorkspace.latest_build.transition is start so Retry will attempt to start
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
    server.use(
      rest.get(`/api/v2/workspaces/${MockWorkspace.id}`, (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockFailedWorkspace))
      }),
    )
    const status = await screen.findByText("Build Failed")
    expect(status).toBeDefined()
    const retryButton = await screen.findByText("Retry")
    retryButton.click()
    const laterStatus = await screen.findByText("Building")
    expect(laterStatus).toBeDefined()
  })
  it("restarts the workspace when the user presses Update", async () => {
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
    server.use(
      rest.get(`/api/v2/workspaces/${MockWorkspace.id}`, (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockOutdatedWorkspace))
      }),
    )
    const updateButton = await screen.findByText("Update")
    updateButton.click()
    const status = await screen.findByText("Building")
    expect(status).toBeDefined()
  })
})
