import { render, screen } from "@testing-library/react"
import React from "react"
import { Workspace } from "./Workspace"
import { MockWorkspace } from "../../test_helpers"

describe("Workspace", () => {
  it("renders", async () => {
    // When
    render(<Workspace workspace={MockWorkspace} />)

    // Then
    const element = await screen.findByText(MockWorkspace.name)
    expect(element).toBeDefined()
  })
})
