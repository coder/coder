import { render, screen } from "@testing-library/react"
import React from "react"
import { MockOrganization, MockProvisioner, MockTemplate } from "./../test_helpers"
import { CreateTemplateForm } from "./CreateTemplateForm"

describe("CreateTemplateForm", () => {
  it("renders", async () => {
    // Given
    const provisioners = [MockProvisioner]
    const organizations = [MockOrganization]
    const onSubmit = () => Promise.resolve(MockTemplate)
    const onCancel = () => Promise.resolve()

    // When
    render(
      <CreateTemplateForm
        provisioners={provisioners}
        organizations={organizations}
        onSubmit={onSubmit}
        onCancel={onCancel}
      />,
    )

    // Then
    // Simple smoke test to verify form renders
    const element = await screen.findByText("Create Template")
    expect(element).toBeDefined()
  })
})
