import { render, screen } from "@testing-library/react"
import React from "react"
import { MockOrganization, MockProject, MockProvisioner } from "./../test_helpers"
import { CreateProjectForm } from "./CreateProjectForm"

describe("CreateProjectForm", () => {
  it("renders", async () => {
    // Given
    const provisioners = [MockProvisioner]
    const organizations = [MockOrganization]
    const onSubmit = () => Promise.resolve(MockProject)
    const onCancel = () => Promise.resolve()

    // When
    render(
      <CreateProjectForm
        provisioners={provisioners}
        organizations={organizations}
        onSubmit={onSubmit}
        onCancel={onCancel}
      />,
    )

    // Then
    // Simple smoke test to verify form renders
    const element = await screen.findByText("Create Project")
    expect(element).toBeDefined()
  })
})
