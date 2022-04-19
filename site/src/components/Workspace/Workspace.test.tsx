import { screen } from "@testing-library/react"
import React from "react"
import { MockOrganization, MockTemplate, MockWorkspace, render } from "../../testHelpers"
import { Workspace } from "./Workspace"

describe("Workspace", () => {
  it("renders", async () => {
    // When
    render(<Workspace organization={MockOrganization} template={MockTemplate} workspace={MockWorkspace} />)

    // Then
    const element = await screen.findByText(MockWorkspace.name)
    expect(element).toBeDefined()
  })
})
