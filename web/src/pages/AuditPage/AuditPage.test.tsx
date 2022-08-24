import { fireEvent, screen } from "@testing-library/react"
import { Language as CopyButtonLanguage } from "components/CopyButton/CopyButton"
import { Language as AuditTooltipLanguage } from "components/Tooltips/AuditHelpTooltip"
import { Language as TooltipLanguage } from "components/Tooltips/HelpTooltip/HelpTooltip"
import { render } from "testHelpers/renderHelpers"
import AuditPage from "./AuditPage"
import { Language as AuditViewLanguage } from "./AuditPageView"

describe("AuditPage", () => {
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

  it("describes the CLI command", async () => {
    // When
    render(<AuditPage />)

    // Then
    await screen.findByText("coder audit [organization_ID]") // CLI command; untranslated
    const copyIcon = await screen.findByRole("button", { name: CopyButtonLanguage.ariaLabel })
    fireEvent.mouseOver(copyIcon)
    expect(await screen.findByText(AuditViewLanguage.tooltipTitle)).toBeInTheDocument()
  })
})
