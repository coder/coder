import { fireEvent, screen } from "@testing-library/react"
import { Language as AuditTooltipLanguage } from "components/Tooltips/AuditHelpTooltip"
import { Language as TooltipLanguage } from "components/Tooltips/HelpTooltip/HelpTooltip"
import { MockAuditLog, MockAuditLog2, render } from "testHelpers/renderHelpers"
import * as CreateDayString from "util/createDayString"
import AuditPage from "./AuditPage"
import { Language as AuditViewLanguage } from "./AuditPageView"

describe("AuditPage", () => {
  beforeEach(() => {
    // Mocking the dayjs module within the createDayString file
    const mock = jest.spyOn(CreateDayString, "createDayString")
    mock.mockImplementation(() => "a minute ago")
  })

  it("renders a page with a title and subtitle", async () => {
    // When
    render(<AuditPage />)

    // Then
    await screen.findByText(AuditViewLanguage.title)
    await screen.findByText(AuditViewLanguage.subtitle)
    const tooltipIcon = await screen.findByRole("button", { name: TooltipLanguage.ariaLabel })
    fireEvent.mouseOver(tooltipIcon)
    expect(await screen.findByText(AuditTooltipLanguage.title)).toBeInTheDocument()
  })

  it("shows the audit logs", async () => {
    // When
    render(<AuditPage />)

    // Then
    await screen.findByTestId(`audit-log-row-${MockAuditLog.id}`)
    screen.getByTestId(`audit-log-row-${MockAuditLog2.id}`)
  })
})
