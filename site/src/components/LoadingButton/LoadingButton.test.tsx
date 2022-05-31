import { render, screen } from "@testing-library/react"
import { LoadingButton } from "./LoadingButton"

describe("LoadingButton", () => {
  it("renders", async () => {
    // When
    render(<LoadingButton>Sign In</LoadingButton>)

    // Then
    const element = await screen.findByText("Sign In")
    expect(element).toBeDefined()
  })

  it("shows spinner if loading is set to true", async () => {
    // When
    render(<LoadingButton loading>Sign in</LoadingButton>)

    // Then
    const spinnerElement = await screen.findByRole("progressbar")
    expect(spinnerElement).toBeDefined()
  })
})
