import { render, screen } from "@testing-library/react"
import React from "react"
import { CreateWorkspaceForm } from "./CreateWorkspaceForm"
import { MockProject, MockWorkspace } from "./../test_helpers"

describe("CreateWorkspaceForm", () => {
  it("renders", async () => {
    // Given
    const onSubmit = () => Promise.resolve(MockWorkspace)
    const onCancel = () => Promise.resolve()

    // When
    render(<CreateWorkspaceForm project={MockProject} onSubmit={onSubmit} onCancel={onCancel} />)

    // Then
    // Simple smoke test to verify form renders
    const element = await screen.findByText("Create Workspace")
    expect(element).toBeDefined()
  })
})
