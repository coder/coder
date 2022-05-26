import { screen } from "@testing-library/react"
import React from "react"
import { MockUser } from "../../testHelpers/entities"
import { render } from "../../testHelpers/renderHelpers"
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
