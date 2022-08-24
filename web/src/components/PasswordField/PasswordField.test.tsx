import { render, screen } from "@testing-library/react"
import { PasswordField } from "./PasswordField"

describe("PasswordField", () => {
  it("renders", async () => {
    // When
    render(<PasswordField helperText="Enter password" />)

    // Then
    const element = await screen.findByText("Enter password")
    expect(element).toBeDefined()
  })
})
