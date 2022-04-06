import { screen } from "@testing-library/react"
import React from "react"
import { render } from "../../test_helpers"
import { MockUser } from "../../test_helpers/entities"
import { UserDropdown, UserDropdownProps } from "./UserDropdown"

const renderAndClick = async (props: Partial<UserDropdownProps> = {}) => {
  render(<UserDropdown user={props.user ?? MockUser} onSignOut={props.onSignOut ?? jest.fn()} />)
  const trigger = await screen.findByTestId("user-dropdown-trigger")
  trigger.click()
}

describe("UserDropdown", () => {
  describe("when the trigger is clicked", () => {
    it("opens the menu", async () => {
      await renderAndClick()
      expect(screen.getByText(/account/i)).toBeDefined()
      expect(screen.getByText(/documentation/i)).toBeDefined()
      expect(screen.getByText(/sign out/i)).toBeDefined()
    })
  })

  describe("when the menu is open", () => {
    describe("and sign out is clicked", () => {
      it("calls the onSignOut function", async () => {
        const onSignOut = jest.fn()
        await renderAndClick({ onSignOut })
        screen.getByText(/sign out/i).click()
        expect(onSignOut).toBeCalledTimes(1)
      })
    })
  })

  it("has the correct link for the documentation item", async () => {
    await renderAndClick()

    const link = screen.getByText(/documentation/i).closest("a")
    if (!link) {
      throw new Error("Anchor tag not found for the documentation menu item")
    }

    expect(link.getAttribute("href")).toBe("https://coder.com/docs")
  })

  it("has the correct link for the account item", async () => {
    await renderAndClick()

    const link = screen.getByText(/account/i).closest("a")
    if (!link) {
      throw new Error("Anchor tag not found for the account menu item")
    }

    expect(link.getAttribute("href")).toBe("/preferences")
  })
})
