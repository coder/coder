import { screen } from "@testing-library/react"
import React from "react"
import { history, render } from "../../testHelpers/renderHelpers"
import { AdminDropdown, Language } from "./AdminDropdown"

const renderAndClick = async () => {
  render(<AdminDropdown />)
  const trigger = await screen.findByText(Language.menuTitle)
  trigger.click()
}

describe("AdminDropdown", () => {
  describe("when the trigger is clicked", () => {
    it("opens the menu", async () => {
      await renderAndClick()
      expect(screen.getByText(Language.usersLabel)).toBeDefined()
      expect(screen.getByText(Language.settingsLabel)).toBeDefined()
    })
  })

  it("links to the users page", async () => {
    await renderAndClick()

    const usersLink = screen.getByText(Language.usersLabel).closest("a")
    usersLink?.click()

    expect(history.location.pathname).toEqual("/users")
  })

  it("links to the settings page", async () => {
    await renderAndClick()

    const usersLink = screen.getByText(Language.settingsLabel).closest("a")
    usersLink?.click()

    expect(history.location.pathname).toEqual("/settings")
  })
})
