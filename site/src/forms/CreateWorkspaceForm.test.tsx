import { render, screen } from "@testing-library/react"
import React from "react"
import { MockTemplate, MockWorkspace } from "./../test_helpers"
import { CreateWorkspaceForm } from "./CreateWorkspaceForm"

describe("CreateWorkspaceForm", () => {
  it("renders", async () => {
    // Given
    const onSubmit = () => Promise.resolve(MockWorkspace)
    const onCancel = () => Promise.resolve()

    // When
    render(<CreateWorkspaceForm template={MockTemplate} onSubmit={onSubmit} onCancel={onCancel} />)

    // Then
    // Simple smoke test to verify form renders
    const element = await screen.findByText("Create Workspace")
    expect(element).toBeDefined()
  })
})
