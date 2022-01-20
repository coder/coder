import React from "react"
import { render, screen } from "@testing-library/react"

import { SignInForm } from "./SignInForm"

describe("SignInForm", () => {
  it("renders content", async () => {
    // When
    render(<SignInForm />)

    // Then
    await screen.findByText("Sign In", { exact: false })
  })

  it("shows an error message if SignIn fails", async () => {
    // Given
    const loginHandler = (_email: string, _password: string) => Promise.reject("Unacceptable credentials")

    // When
    // Render the component
    render(<SignInForm loginHandler={loginHandler} />)
    // Click sign-in
    const elem = await screen.findByRole("button")
    elem.click()

    // Then
    // Should see an error message
    const errorMessage = await screen.findByText("The username or password is incorrect.")
    expect(errorMessage).toBeDefined()
  })

  it("calls on login success when login completes", async () => {
    // Given
    const loginHandler = (_email: string, _password: string) => Promise.resolve()
    const onLoginSuccess = jest.fn()

    // When
    // Render the component
    render(<SignInForm loginHandler={loginHandler} onLoginSuccess={onLoginSuccess} />)
    // Click sign-in
    const elem = await screen.findByRole("button")
    elem.click()

    // Then
    expect(onLoginSuccess).toHaveBeenCalledTimes(1)
  })
})
