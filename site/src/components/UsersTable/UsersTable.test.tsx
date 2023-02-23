import { fireEvent, screen } from "@testing-library/react"
import { Language as TooltipLanguage } from "components/Tooltips/HelpTooltip/HelpTooltip"
import { Language as UserRoleLanguage } from "components/Tooltips/UserRoleHelpTooltip"
import { render } from "testHelpers/renderHelpers"
import { UsersTable } from "./UsersTable"

describe("AuditPage", () => {
  it("renders a page with a title and subtitle", async () => {
    // When
    render(
      <UsersTable
        onSuspendUser={() => jest.fn()}
        onDeleteUser={() => jest.fn()}
        onListWorkspaces={() => jest.fn()}
        onActivateUser={() => jest.fn()}
        onResetUserPassword={() => jest.fn()}
        onUpdateUserRoles={() => jest.fn()}
        isNonInitialPage={false}
        actorID="12345678-1234-1234-1234-123456789012"
      />,
    )

    // Then
    const tooltipIcon = await screen.findByRole("button", {
      name: TooltipLanguage.ariaLabel,
    })
    fireEvent.mouseOver(tooltipIcon)
    expect(await screen.findByText(UserRoleLanguage.title)).toBeInTheDocument()
  })
})
