import { fireEvent, screen } from "@testing-library/react"
import { MockUser } from "../../testHelpers/entities"
import { render } from "../../testHelpers/renderHelpers"
import { Language } from "../UserDropdownContent/UserDropdownContent"
import { UserDropdown, UserDropdownProps } from "./UsersDropdown"

const renderAndClick = async (props: Partial<UserDropdownProps> = {}) => {
  render(<UserDropdown user={props.user ?? MockUser} onSignOut={props.onSignOut ?? jest.fn()} />)
  const trigger = await screen.findByTestId("user-dropdown-trigger")
  fireEvent.click(trigger)
}

describe("UserDropdown", () => {
  describe("when the trigger is clicked", () => {
    it("opens the menu", async () => {
      await renderAndClick()
      expect(screen.getByText(Language.accountLabel)).toBeDefined()
      expect(screen.getByText(Language.docsLabel)).toBeDefined()
      expect(screen.getByText(Language.signOutLabel)).toBeDefined()
    })
  })
})
