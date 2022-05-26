import { screen } from "@testing-library/react"
import React from "react"
import { MockUser } from "../../testHelpers/entities"
import { render } from "../../testHelpers/renderHelpers"
import { Language as navLanguage, NavbarView } from "./NavbarView"

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

  it("workspaces nav link has the correct href", async () => {
    render(<NavbarView user={MockUser} onSignOut={noop} />)
    const workspacesLink = await screen.findByText(navLanguage.workspaces)
    expect((workspacesLink as HTMLAnchorElement).href).toContain("/workspaces")
  })

  it("templates nav link has the correct href", async () => {
    render(<NavbarView user={MockUser} onSignOut={noop} />)
    const templatesLink = await screen.findByText(navLanguage.templates)
    expect((templatesLink as HTMLAnchorElement).href).toContain("/templates")
  })

  it("users nav link has the correct href", async () => {
    render(<NavbarView user={MockUser} onSignOut={noop} />)
    const userLink = await screen.findByText(navLanguage.users)
    expect((userLink as HTMLAnchorElement).href).toContain("/users")
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
