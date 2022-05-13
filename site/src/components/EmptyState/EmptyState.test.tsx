import { screen } from "@testing-library/react"
import React from "react"
import { render } from "../../testHelpers/renderHelpers"
import { EmptyState } from "./EmptyState"

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
