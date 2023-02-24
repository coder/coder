import { screen } from "@testing-library/react"
import { MockBuildInfo, MockSupportLinks, MockUser } from "../../testHelpers/entities"
import { render } from "../../testHelpers/renderHelpers"
import { Language, UserDropdownContent } from "./UserDropdownContent"

describe("UserDropdownContent", () => {
  const env = process.env

  // REMARK: copying process.env so we don't mutate that object or encounter conflicts between tests
  beforeEach(() => {
    process.env = { ...env }
  })

  // REMARK: restoring process.env
  afterEach(() => {
    process.env = env
  })

  it("displays the menu items", () => {
    render(
      <UserDropdownContent
        user={MockUser}
        buildInfo={MockBuildInfo}
        supportLinks={MockSupportLinks}
        onSignOut={jest.fn()}
        onPopoverClose={jest.fn()}
      />,
    )
    expect(screen.getByText(Language.accountLabel)).toBeDefined()
    expect(screen.getByText(Language.signOutLabel)).toBeDefined()
    expect(screen.getByText(Language.copyrightText)).toBeDefined()
    expect(screen.getByText(MockSupportLinks[0].name)).toBeDefined()
    expect(screen.getByText(MockSupportLinks[1].name)).toBeDefined()
    expect(screen.getByText(MockSupportLinks[2].name)).toBeDefined()
    expect(screen.getByText(MockBuildInfo.version)).toBeDefined()
  })

  it("has the correct link for the account item", () => {
    render(
      <UserDropdownContent
        user={MockUser}
        onSignOut={jest.fn()}
        onPopoverClose={jest.fn()}
      />,
    )

    const link = screen.getByText(Language.accountLabel).closest("a")
    if (!link) {
      throw new Error("Anchor tag not found for the account menu item")
    }

    expect(link.getAttribute("href")).toBe("/settings/account")
  })

  it("calls the onSignOut function", () => {
    const onSignOut = jest.fn()
    render(
      <UserDropdownContent
        user={MockUser}
        onSignOut={onSignOut}
        onPopoverClose={jest.fn()}
      />,
    )
    screen.getByText(Language.signOutLabel).click()
    expect(onSignOut).toBeCalledTimes(1)
  })
})
