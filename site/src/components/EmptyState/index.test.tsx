import { screen } from "@testing-library/react"
import { render } from "../../test_helpers"
import React from "react"
import { EmptyState } from "./index"

describe("EmptyState", () => {
  it("renders (smoke test)", async () => {
    // When
    render(<EmptyState message="Hello, world" />)

    // Then
    await screen.findByText("Hello, world")
  })

  it("renders description text", async () => {
    // When
    render(<EmptyState message="Hello, world" description="Friendly greeting" />)

    // Then
    await screen.findByText("Hello, world")
    await screen.findByText("Friendly greeting")
  })

  it("renders description component", async () => {
    // Given
    const description = <button title="Click me" />

    // When
    render(<EmptyState message="Hello, world" description={description} />)

    // Then
    await screen.findByText("Hello, world")
    await screen.findByRole("button")
  })
})
