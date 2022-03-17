import React from "react"
import { act, fireEvent, screen, waitFor } from "@testing-library/react"
import { history, render } from "../test_helpers"
import { SignInPage } from "./login"
import { server } from "../mocks/server"
import { rest } from "msw"

describe("SignInPage", () => {
  beforeEach(() => {
    history.replace("/login")
  })

  it("renders the sign-in form", async () => {
    // When
    render(<SignInPage />)

    // Then
    await screen.findByText("Sign In", { exact: false })
  })

  it("shows an error message if SignIn fails", async () => {
    // Given
    const { container } = render(<SignInPage />)
    // Make login fail
    server.use(
      rest.post('/api/v2/users/login', async (req, res, ctx) => {
        return res(
          ctx.status(500), 
          ctx.json({message: 'nope'}))
      }),
    )

    // When
    // Set username / password
    const [username, password] = container.querySelectorAll("input")
    fireEvent.change(username, { target: { value: "test@coder.com" } })
    fireEvent.change(password, { target: { value: "password" } })
    // Click sign-in
    const signInButton = await screen.findByText("Sign In")
    act(() => signInButton.click())

    // Then
    // Finding error by test id because it comes from the backend
    const errorMessage = await screen.findByTestId("sign-in-error")
    expect(errorMessage).toBeDefined()
    expect(history.location.pathname).toEqual("/login")
  })

  it("redirects when login is complete", async () => {
    // Given
    const { container } = render(<SignInPage />)

    // When user signs in
    const [username, password] = container.querySelectorAll("input")
    fireEvent.change(username, { target: { value: "test@coder.com" } })
    fireEvent.change(password, { target: { value: "password" } })
    const signInButton = await screen.findByText("Sign In")
    act(() => signInButton.click())

    // Then
    await waitFor(() => expect(history.location.pathname).toEqual("/projects"))
  })

  it("respects ?redirect query parameter when complete", async () => {
    // Given
    const { container } = render(<SignInPage />)
    // Set a path to redirect to after login is successful
    act(() => history.replace("/login?redirect=%2Fsome%2Fother%2Fpath"))
    console.log(history.location.pathname)

    // When user signs in
    const [username, password] = container.querySelectorAll("input")
    fireEvent.change(username, { target: { value: "test@coder.com" } })
    fireEvent.change(password, { target: { value: "password" } })
    const signInButton = await screen.findByText("Sign In")
    act(() => signInButton.click())

    // Then
    await waitFor(() => expect(history.location.pathname).toEqual("/some/other/path"))
  })
})
