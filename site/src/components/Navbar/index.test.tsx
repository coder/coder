import React from "react"
import { screen } from "@testing-library/react"

import { render, MockUser } from "../../test_helpers"
import { Navbar } from "./index"

describe("Navbar", () => {
  const noop = () => {
    return
  }
  it("renders content", async () => {
    // When
    render(<Navbar onSignOut={noop} />)

    // Then
    await screen.findAllByText("Coder", { exact: false })
  })

  it("renders profile picture for user", async () => {
    // Given
    const mockUser = {
      ...MockUser,
      username: "bryan",
    }

    // When
    render(<Navbar user={mockUser} onSignOut={noop} />)

    // Then
    // There should be a 'B' avatar!
    const element = await screen.findByText("B")
    expect(element).toBeDefined()
  })
})
