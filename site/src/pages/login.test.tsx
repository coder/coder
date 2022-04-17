import { act, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { rest } from "msw"
import React from "react"
import { Language } from "../components/SignIn/SignInForm"
import { history, render } from "../test_helpers"
import { server } from "../test_helpers/server"
import { SignInPage } from "./login"

describe("SignInPage", () => {
  beforeEach(() => {
    history.replace("/login")
    // appear logged out
    server.use(
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        return res(ctx.status(401), ctx.json({ message: "no user here" }))
      }),
    )
    // only leave password auth enabled by default
    server.use(
      rest.get("/api/v2/users/auth", (req, res, ctx) => {
        return res(
          ctx.status(200),
          ctx.json({
            password: true,
            github: false,
          }),
        )
      }),
    )
  })

  it("renders the sign-in form", async () => {
    // When
    render(<SignInPage />)

    // Then
    await screen.findByText(Language.basicSignIn)
  })

  it("shows an error message if SignIn fails", async () => {
    // Given
    server.use(
      // Make login fail
      rest.post("/api/v2/users/login", async (req, res, ctx) => {
        return res(ctx.status(500), ctx.json({ message: "nope" }))
      }),
    )

    // When
    render(<SignInPage />)
    const email = screen.getByLabelText(Language.emailLabel)
    const password = screen.getByLabelText(Language.passwordLabel)
    await userEvent.type(email, "test@coder.com")
    await userEvent.type(password, "password")
    // Click sign-in
    const signInButton = await screen.findByText(Language.basicSignIn)
    act(() => signInButton.click())

    // Then
    const errorMessage = await screen.findByText(Language.authErrorMessage)
    expect(errorMessage).toBeDefined()
    expect(history.location.pathname).toEqual("/login")
  })

  it("shows github authentication when enabled", async () => {
    // Given
    server.use(
      rest.get("/api/v2/users/auth", async (req, res, ctx) => {
        return res(
          ctx.status(200),
          ctx.json({
            password: true,
            github: true,
          }),
        )
      }),
    )

    // When
    render(<SignInPage />)

    // Then
    await screen.findByText(Language.githubSignIn)
  })
})
