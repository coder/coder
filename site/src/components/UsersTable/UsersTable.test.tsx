import { fireEvent, screen } from "@testing-library/react"
import { Language as UserRoleLanguage } from "components/Tooltips/UserRoleHelpTooltip"
import { Language as TooltipLanguage } from "components/Tooltips/HelpTooltip/HelpTooltip"
import { render } from "testHelpers/renderHelpers"
import { UsersTable } from "./UsersTable"

describe("AuditPage", () => {
  it("renders a page with a title and subtitle", async () => {
    // When
    render(<UsersTable
      onSuspendUser={() => jest.fn()}
      onActivateUser={() => jest.fn()}
      onResetUserPassword={() => jest.fn()}
      onUpdateUserRoles={() => jest.fn()}
    />)

    // Then
    const tooltipIcon = await screen.findByRole("button", { name: TooltipLanguage.ariaLabel })
    fireEvent.mouseOver(tooltipIcon)
    expect(await screen.findByText(UserRoleLanguage.title)).toBeInTheDocument()
  })
})
