import React from "react"
import { screen } from "@testing-library/react"
import { MockTemplate, MockWorkspace, render, history } from "../../testHelpers"
import { WorkspacePage } from "./WorkspacePage"

describe("Workspace Page", () => {
  beforeEach(() => {
    history.replace(`/workspace/${MockWorkspace.id}`)
  })
  it.only("shows a workspace", async () => {
    render(<WorkspacePage />)
    const workspaceName = await screen.findByText(MockWorkspace.name)
    const templateName = await screen.findByText(MockTemplate.name)
    expect(workspaceName).toBeDefined()
    expect(templateName).toBeDefined()
  })
})
