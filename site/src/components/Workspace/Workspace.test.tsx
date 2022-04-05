import { screen } from "@testing-library/react"
import React from "react"
import { MockOrganization, MockProject, MockWorkspace, render } from "../../test_helpers"
import { Workspace } from "./Workspace"

describe("Workspace", () => {
  it("renders", async () => {
    // When
    render(<Workspace organization={MockOrganization} project={MockProject} workspace={MockWorkspace} />)

    // Then
    const element = await screen.findByText(MockWorkspace.name)
    expect(element).toBeDefined()
  })
})
