import React from "react"
import { screen } from "@testing-library/react"
import { MockTemplate, MockWorkspace, render, history, renderWithAuth } from "../../testHelpers"
import { WorkspacePage } from "./WorkspacePage"

describe("Workspace Page", () => {
  it("shows a workspace", async () => {
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace"})
    const workspaceName = await screen.findByText(MockWorkspace.name)
    const templateName = await screen.findByText(MockTemplate.name)
    expect(workspaceName).toBeDefined()
    expect(templateName).toBeDefined()
  })
})
