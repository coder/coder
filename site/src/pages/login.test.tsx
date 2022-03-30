import React from "react"
import { act, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { history, render } from "../test_helpers"
import { SignInPage } from "./login"
import { server } from "../test_helpers/server"
import { rest } from "msw"
import { Language } from "../components/SignIn/SignInForm"

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
    await screen.findByText(Language.signIn, { exact: false })
  })

  it("shows an error message if SignIn fails", async () => {
    // Given
    render(<SignInPage />)
    // Make login fail
    server.use(
      rest.post("/api/v2/users/login", async (req, res, ctx) => {
        return res(ctx.status(500), ctx.json({ message: "nope" }))
      }),
    )

    // When
    // Set email / password
    const email = screen.getByLabelText(Language.emailLabel)
    const password = screen.getByLabelText(Language.passwordLabel)
    userEvent.type(email, "test@coder.com")
    userEvent.type(password, "password")
    // Click sign-in
    const signInButton = await screen.findByText(Language.signIn)
    act(() => signInButton.click())

    // Then
    // Finding error by test id because it comes from the backend
    const errorMessage = await screen.findByText(Language.authErrorMessage)
    expect(errorMessage).toBeDefined()
    expect(history.location.pathname).toEqual("/login")
  })
})
