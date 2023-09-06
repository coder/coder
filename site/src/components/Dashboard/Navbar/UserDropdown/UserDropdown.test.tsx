import { fireEvent, screen } from "@testing-library/react"
import { MockSupportLinks, MockUser } from "../../../../testHelpers/entities"
import { render } from "../../../../testHelpers/renderHelpers"
import { Language } from "./UserDropdownContent/UserDropdownContent"
import { UserDropdown, UserDropdownProps } from "./UserDropdown"

const renderAndClick = async (props: Partial<UserDropdownProps> = {}) => {
  render(
    <UserDropdown
      user={props.user ?? MockUser}
      supportLinks={MockSupportLinks}
      onSignOut={props.onSignOut ?? jest.fn()}
    />,
  )
  const trigger = await screen.findByTestId("user-dropdown-trigger")
  fireEvent.click(trigger)
}

describe("UserDropdown", () => {
  describe("when the trigger is clicked", () => {
    it("opens the menu", async () => {
      await renderAndClick()
      expect(screen.getByText(Language.accountLabel)).toBeDefined()
      expect(screen.getByText(MockSupportLinks[0].name)).toBeDefined()
      expect(screen.getByText(MockSupportLinks[1].name)).toBeDefined()
      expect(screen.getByText(MockSupportLinks[2].name)).toBeDefined()
      expect(screen.getByText(Language.signOutLabel)).toBeDefined()
    })
  })
})
