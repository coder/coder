import { screen } from "@testing-library/react"
import React from "react"
import { MockTemplate, MockWorkspace, renderWithAuth } from "../../testHelpers"
import { WorkspacePage } from "./WorkspacePage"

describe("Workspace Page", () => {
  it("shows a workspace", async () => {
    renderWithAuth(<WorkspacePage />, { route: `/workspaces/${MockWorkspace.id}`, path: "/workspaces/:workspace" })
    const workspaceName = await screen.findByText(MockWorkspace.name)
    const templateName = await screen.findByText(MockTemplate.name)
    expect(workspaceName).toBeDefined()
    expect(templateName).toBeDefined()
  })
})
