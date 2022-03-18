import React from "react"
import { act, fireEvent, screen } from "@testing-library/react"
import { history, render } from "../test_helpers"
import { SignInPage } from "./login"
import { server } from "../test_helpers/server"
import { rest } from "msw"

describe("SignInPage", () => {
  beforeEach(() => {
    history.replace("/login")
    // appear logged out
    server.use(
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        return res(ctx.status(401), ctx.json({ message: "no user here" }))
      }),
    )
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
      rest.post("/api/v2/users/login", async (req, res, ctx) => {
        return res(ctx.status(500), ctx.json({ message: "nope" }))
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
})
