import { screen } from "@testing-library/react"
import { MockBuildInfo, render } from "../../testHelpers/renderHelpers"
import { Footer, Language } from "./Footer"

describe("Footer", () => {
  it("renders content", async () => {
    // When
    render(<Footer buildInfo={MockBuildInfo} />)

    // Then
    await screen.findByText("Copyright", { exact: false })
    await screen.findByText(Language.buildInfoText(MockBuildInfo))
    const reportBugLink = screen
      .getByText(Language.reportBugLink, { exact: false })
      .closest("a")
    if (!reportBugLink) {
      throw new Error("Bug report link not found in footer")
    }

    expect(reportBugLink.getAttribute("href")?.length).toBeGreaterThan(0)
  })
})
