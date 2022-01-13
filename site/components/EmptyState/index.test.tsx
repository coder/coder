import { screen } from "@testing-library/react"
import { render } from "../../test_helpers"
import React from "react"
import { EmptyState, EmptyStateProps } from "./index"

describe("EmptyState", () => {
  it("renders (smoke test)", async () => {
    // When
    render(<EmptyState message="Hello, world" />)

    // Then
    await screen.findByText("Hello, world")
  })
})
