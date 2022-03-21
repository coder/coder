import React from "react"
import { screen } from "@testing-library/react"

import { render } from "../../test_helpers"
import { MockUser } from "../../test_helpers/entities"
import { NavbarView } from "./NavbarView"

describe("NavbarView", () => {
  const noop = () => {
    return
  }
  it("renders content", async () => {
    // When
    render(<NavbarView user={MockUser} onSignOut={noop} />)

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
    render(<NavbarView user={mockUser} onSignOut={noop} />)

    // Then
    // There should be a 'B' avatar!
    const element = await screen.findByText("B")
    expect(element).toBeDefined()
  })
})
