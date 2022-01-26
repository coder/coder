import { render, screen } from "@testing-library/react"
import React from "react"
import { CreateWorkspaceForm } from "./CreateWorkspaceForm"
import { MockProvisioner, MockOrganization, MockProject, MockWorkspace } from "./../test_helpers"

describe("CreateProjectForm", () => {
  it("renders", async () => {
    // Given
    const project = MockProject
    const onSubmit = () => Promise.resolve(MockWorkspace)
    const onCancel = () => Promise.resolve()

    // When
    render(
      <CreateWorkspaceForm
        project={project}
        onSubmit={onSubmit}
        onCancel={onCancel}
      />,
    )

    // Then
    // Simple smoke test to verify form renders
    const element = await screen.findByText("Create Workspace")
    expect(element).toBeDefined()
  })
})
