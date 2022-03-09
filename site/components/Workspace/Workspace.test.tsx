import { render, screen } from "@testing-library/react"
import React from "react"
import { Workspace } from "./Workspace"
import { MockOrganization, MockProject, MockWorkspace } from "../../test_helpers"

describe("Workspace", () => {
  it("renders", async () => {
    // When
    render(<Workspace organization={MockOrganization} project={MockProject} workspace={MockWorkspace} />)

    // Then
    const element = await screen.findByText(MockWorkspace.name)
    expect(element).toBeDefined()
  })
})
