import { render, screen } from "@testing-library/react"
import { ErrorSummary } from "./ErrorSummary"

describe("ErrorSummary", () => {
  it("renders", async () => {
    // When
    const error = new Error("test error message")
    render(<ErrorSummary error={error} />)

    // Then
    const element = await screen.findByText("test error message", { exact: false })
    expect(element).toBeDefined()
  })
})
