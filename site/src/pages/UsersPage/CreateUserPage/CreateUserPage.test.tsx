import { act, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { rest } from "msw"
import React from "react"
import { Language as FormLanguage } from "../../../components/CreateUserForm/CreateUserForm"
import { Language as FooterLanguage } from "../../../components/FormFooter/FormFooter"
import { Language as UserLanguage } from "../../../xServices/users/usersXService"
import { history, render } from "../../../testHelpers"
import { server } from "../../../testHelpers/server"
import { CreateUserPage, Language } from "./CreateUserPage"

const fillForm = async ({
  username = "testuser",
  email = "test@coder.com",
  password = "password",
}: {
  username?: string
  email?: string
  password?: string
}) => {
  const usernameField = screen.getByLabelText(FormLanguage.usernameLabel)
  const emailField = screen.getByLabelText(FormLanguage.emailLabel)
  const passwordField = screen.getByLabelText(FormLanguage.passwordLabel)
  await userEvent.type(usernameField, username)
  await userEvent.type(emailField, email)
  await userEvent.type(passwordField, password)
  const submitButton = await screen.findByText(FooterLanguage.defaultSubmitLabel)
  act(() => submitButton.click())
}

describe("Create User Page", () => {
  beforeEach(() => {
    history.replace("/users/create")
  })

  it("shows validation error message", async () => {
    render(<CreateUserPage />)
    await fillForm({ email: "test" })
    const errorMessage = await screen.findByText(FormLanguage.emailInvalid)
    expect(errorMessage).toBeDefined()
  })
  it("shows generic error message", async () => {
    server.use(
      rest.post("/api/v2/users", (req, res, ctx) => {
        Promise.reject("something went wrong")
      }),
    )
    render(<CreateUserPage />)
    await fillForm({})
    const errorMessage = await screen.findByText(Language.unknownError)
    expect(errorMessage).toBeDefined()
  })
  it("shows API error message", async () => {
    const fieldErrorMessage = "username already in use"
    server.use(
      rest.post("/api/v2/users", (req, res, ctx) => {
        return res(ctx.status(400), ctx.json({
          message: "invalid field",
          errors: [{
            detail: fieldErrorMessage,
            field: "username"
          }]
        }))
      }),
    )
    render(<CreateUserPage />)
    await fillForm({})
    const errorMessage = await screen.findByText(fieldErrorMessage)
    expect(errorMessage).toBeDefined()
  })
  it("shows success notification and redirects to users page", async () => {
    render(<CreateUserPage />)
    await fillForm({})
    const successMessage = screen.findByText(UserLanguage.createUserSuccess)
    expect(successMessage).toBeDefined()
    expect(history.location.pathname).toEqual("/users")
  })
  it("redirects to users page on cancel", () => {
    render(<CreateUserPage />)
    screen.findByText(FooterLanguage.cancelLabel)
    expect(history.location.pathname).toEqual("/users")
  })
  it("redirects to users page on close", () => {
    render(<CreateUserPage />)
    screen.findByText("ESC")
    expect(history.location.pathname).toEqual("/users")
  })
})
