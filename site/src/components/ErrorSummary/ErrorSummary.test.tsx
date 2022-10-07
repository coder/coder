import { fireEvent, render, screen } from "@testing-library/react"
import { ErrorSummary } from "./ErrorSummary"
import { i18n } from "i18n"

const { t } = i18n

describe("ErrorSummary", () => {
  it("renders", async () => {
    // When
    const error = new Error("test error message")
    render(<ErrorSummary error={error} />)

    // Then
    const element = await screen.findByText("test error message")
    expect(element).toBeDefined()
  })

  it("shows details on More click", async () => {
    // When
    const error = {
      response: {
        data: {
          message: "Failed to fetch something!",
          detail: "The resource you requested does not exist in the database.",
        },
      },
      isAxiosError: true,
    }
    render(<ErrorSummary error={error} />)

    // Then
    const expandText = t("ctas.expand", { ns: "common" })
    fireEvent.click(screen.getByText(expandText))
    const element = await screen.findByText(
      "The resource you requested does not exist in the database.",
      { exact: false },
    )
    expect(element.closest(".MuiCollapse-entered")).toBeDefined()
  })

  it("hides details on Less click", async () => {
    // When
    const error = {
      response: {
        data: {
          message: "Failed to fetch something!",
          detail: "The resource you requested does not exist in the database.",
        },
      },
      isAxiosError: true,
    }
    render(<ErrorSummary error={error} />)

    // Then
    const expandText = t("ctas.expand", { ns: "common" })
    const collapseText = t("ctas.collapse", { ns: "common" })

    fireEvent.click(screen.getByText(expandText))
    fireEvent.click(screen.getByText(collapseText))
    const element = await screen.findByText(
      "The resource you requested does not exist in the database.",
      { exact: false },
    )
    expect(element.closest(".MuiCollapse-hidden")).toBeDefined()
  })

  it("renders nothing on closing", async () => {
    // When
    const error = new Error("test error message")
    render(<ErrorSummary error={error} dismissible />)

    // Then
    const element = await screen.findByText("test error message")
    expect(element).toBeDefined()

    const closeIcon = screen.getAllByRole("button")[0]
    fireEvent.click(closeIcon)
    const nullElement = screen.queryByText("test error message")
    expect(nullElement).toBeNull()
  })
})
