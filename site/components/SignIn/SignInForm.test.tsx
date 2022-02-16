import React from "react"
import singletonRouter from "next/router"
import mockRouter from "next-router-mock"
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react"

import { SignInForm } from "./SignInForm"

describe("SignInForm", () => {
  beforeEach(() => {
    mockRouter.setCurrentUrl("/login")
  })

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
    const { container } = render(<SignInForm loginHandler={loginHandler} />)
    const inputs = container.querySelectorAll("input")
    // Set username / password
    fireEvent.change(inputs[0], { target: { value: "test@coder.com" } })
    fireEvent.change(inputs[1], { target: { value: "password" } })
    // Click sign-in
    const elem = await screen.findByText("Sign In")
    act(() => elem.click())

    // Then
    // Should see an error message
    const errorMessage = await screen.findByText("The username or password is incorrect.")
    expect(errorMessage).toBeDefined()
  })

  it("redirects when login is complete", async () => {
    // Given
    const loginHandler = (_email: string, _password: string) => Promise.resolve()

    // When
    // Render the component
    const { container } = render(<SignInForm loginHandler={loginHandler} />)
    // Set user / password
    const inputs = container.querySelectorAll("input")
    fireEvent.change(inputs[0], { target: { value: "test@coder.com" } })
    fireEvent.change(inputs[1], { target: { value: "password" } })
    // Click sign-in
    const elem = await screen.findByText("Sign In")
    act(() => elem.click())

    // Then
    // Should redirect because login was successful
    await waitFor(() => expect(singletonRouter).toMatchObject({ asPath: "/" }))
  })

  it("respects ?redirect query parameter when complete", async () => {
    // Given
    const loginHandler = (_email: string, _password: string) => Promise.resolve()
    // Set a path to redirect to after login is successful
    mockRouter.setCurrentUrl("/login?redirect=%2Fsome%2Fother%2Fpath")

    // When
    // Render the component
    const { container } = render(<SignInForm loginHandler={loginHandler} />)
    // Set user / password
    const inputs = container.querySelectorAll("input")
    fireEvent.change(inputs[0], { target: { value: "test@coder.com" } })
    fireEvent.change(inputs[1], { target: { value: "password" } })
    // Click sign-in
    const elem = await screen.findByText("Sign In")
    act(() => elem.click())

    // Then
    // Should redirect to /some/other/path because ?redirect was specified and login was successful
    await waitFor(() => expect(singletonRouter).toMatchObject({ asPath: "/some/other/path" }))
  })
})
