import { fireEvent, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import * as API from "api/api"
import { rest } from "msw"
import { history, MockUser, render } from "testHelpers/renderHelpers"
import { server } from "testHelpers/server"
import { Language as SetupLanguage } from "xServices/setup/setupXService"
import { SetupPage } from "./SetupPage"
import { Language as PageViewLanguage } from "./SetupPageView"

const fillForm = async ({
  username = "someuser",
  email = "someone@coder.com",
  password = "password",
}: {
  username?: string
  email?: string
  password?: string
} = {}) => {
  const usernameField = screen.getByLabelText(PageViewLanguage.usernameLabel)
  const emailField = screen.getByLabelText(PageViewLanguage.emailLabel)
  const passwordField = screen.getByLabelText(PageViewLanguage.passwordLabel)
  await userEvent.type(usernameField, username)
  await userEvent.type(emailField, email)
  await userEvent.type(passwordField, password)
  const submitButton = screen.getByRole("button", {
    name: PageViewLanguage.create,
  })
  fireEvent.click(submitButton)
}

describe("Setup Page", () => {
  beforeEach(() => {
    history.replace("/setup")
    // appear logged out
    server.use(
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        return res(ctx.status(401), ctx.json({ message: "no user here" }))
      }),
    )
  })

  it("shows validation error message", async () => {
    render(<SetupPage />)
    await fillForm({ email: "test" })
    const errorMessage = await screen.findByText(PageViewLanguage.emailInvalid)
    expect(errorMessage).toBeDefined()
  })

  it("shows generic error message", async () => {
    jest.spyOn(API, "createFirstUser").mockRejectedValueOnce({
      data: "unknown error",
    })
    render(<SetupPage />)
    await fillForm()
    const errorMessage = await screen.findByText(
      SetupLanguage.createFirstUserError,
    )
    expect(errorMessage).toBeDefined()
  })

  it("shows API error message", async () => {
    const fieldErrorMessage = "invalid username"
    server.use(
      rest.post("/api/v2/users/first", async (req, res, ctx) => {
        return res(
          ctx.status(400),
          ctx.json({
            message: "invalid field",
            validations: [
              {
                detail: fieldErrorMessage,
                field: "username",
              },
            ],
          }),
        )
      }),
    )
    render(<SetupPage />)
    await fillForm()
    const errorMessage = await screen.findByText(fieldErrorMessage)
    expect(errorMessage).toBeDefined()
  })

  it("redirects to workspaces page when success", async () => {
    render(<SetupPage />)

    // simulates the user will be authenticated
    server.use(
      rest.get("/api/v2/users/me", (req, res, ctx) => {
        return res(ctx.status(200), ctx.json(MockUser))
      }),
    )

    await fillForm()
    await waitFor(() =>
      expect(history.location.pathname).toEqual("/workspaces"),
    )
  })
})
