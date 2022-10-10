import { screen } from "@testing-library/react"
import { render } from "../../testHelpers/renderHelpers"
import { Language as ButtonLanguage } from "./createCtas"
import {
  Language as RuntimeErrorStateLanguage,
  RuntimeErrorState,
} from "./RuntimeErrorState"

describe("RuntimeErrorState", () => {
  beforeEach(() => {
    // Given
    const errorText = "broken!"
    const errorStateProps = {
      error: new Error(errorText),
    }

    // When
    render(<RuntimeErrorState {...errorStateProps} />)
  })

  it("should show stack when encountering runtime error", () => {
    // Then
    const reportError = screen.getByText("broken!")
    expect(reportError).toBeDefined()

    // Despite appearances, this is the stack trace
    const stackTrace = screen.getByText("Unable to get stack trace")
    expect(stackTrace).toBeDefined()
  })

  it("should have a button bar", () => {
    // Then
    const copyCta = screen.getByText(ButtonLanguage.copyReport)
    expect(copyCta).toBeDefined()

    const reloadCta = screen.getByText(ButtonLanguage.reloadApp)
    expect(reloadCta).toBeDefined()
  })

  it("should have an email link", () => {
    // Then
    const emailLink = screen.getByText(RuntimeErrorStateLanguage.link)
    expect(emailLink.closest("a")).toHaveAttribute(
      "href",
      expect.stringContaining("mailto:support@coder.com"),
    )
  })
})
