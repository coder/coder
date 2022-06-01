import { screen } from "@testing-library/react"
import { MockAdminRole, MockUser } from "../../testHelpers/entities"
import { render } from "../../testHelpers/renderHelpers"
import { Language, UserDropdownContent } from "./UserDropdownContent"

describe("UserProfileCard", () => {
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
    render(<UserDropdownContent user={MockUser} onSignOut={jest.fn()} onPopoverClose={jest.fn()} />)
    expect(screen.getByText(Language.accountLabel)).toBeDefined()
    expect(screen.getByText(Language.docsLabel)).toBeDefined()
    expect(screen.getByText(Language.signOutLabel)).toBeDefined()
  })

  it("displays the user's roles", () => {
    render(<UserDropdownContent user={MockUser} onSignOut={jest.fn()} onPopoverClose={jest.fn()} />)

    expect(screen.getByText(MockAdminRole.display_name)).toBeDefined()
  })

  it("has the correct link for the documentation item", () => {
    process.env.CODER_VERSION = "v0.5.4"
    render(<UserDropdownContent user={MockUser} onSignOut={jest.fn()} onPopoverClose={jest.fn()} />)

    const link = screen.getByText(Language.docsLabel).closest("a")
    if (!link) {
      throw new Error("Anchor tag not found for the documentation menu item")
    }

    expect(link.getAttribute("href")).toBe(`https://github.com/coder/coder/tree/${process.env.CODER_VERSION}/docs`)
  })

  it("has the correct link for the account item", () => {
    render(<UserDropdownContent user={MockUser} onSignOut={jest.fn()} onPopoverClose={jest.fn()} />)

    const link = screen.getByText(Language.accountLabel).closest("a")
    if (!link) {
      throw new Error("Anchor tag not found for the account menu item")
    }

    expect(link.getAttribute("href")).toBe("/settings/account")
  })

  it("calls the onSignOut function", () => {
    const onSignOut = jest.fn()
    render(<UserDropdownContent user={MockUser} onSignOut={onSignOut} onPopoverClose={jest.fn()} />)
    screen.getByText(Language.signOutLabel).click()
    expect(onSignOut).toBeCalledTimes(1)
  })
})
