import { fireEvent, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { rest } from "msw"
import * as API from "../../../api/api"
import { Language as FormLanguage } from "../../../components/CreateUserForm/CreateUserForm"
import { Language as FooterLanguage } from "../../../components/FormFooter/FormFooter"
import {
  history,
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "../../../testHelpers/renderHelpers"
import { server } from "../../../testHelpers/server"
import { Language as CreateUserLanguage } from "../../../xServices/users/createUserXService"
import { CreateUserPage } from "./CreateUserPage"

const renderCreateUserPage = async () => {
  renderWithAuth(<CreateUserPage />)
  await waitForLoaderToBeRemoved()
}

const fillForm = async ({
  username = "someuser",
  email = "someone@coder.com",
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
  const submitButton = await screen.findByText(
    FooterLanguage.defaultSubmitLabel,
  )
  fireEvent.click(submitButton)
}

describe("Create User Page", () => {
  beforeEach(() => {
    history.replace("/users/create")
  })

  it("shows validation error message", async () => {
    await renderCreateUserPage()
    await fillForm({ email: "test" })
    const errorMessage = await screen.findByText(FormLanguage.emailInvalid)
    expect(errorMessage).toBeDefined()
  })

  it("shows generic error message", async () => {
    jest.spyOn(API, "createUser").mockRejectedValueOnce({
      data: "unknown error",
    })
    await renderCreateUserPage()
    await fillForm({})
    const errorMessage = await screen.findByText(
      CreateUserLanguage.createUserError,
    )
    expect(errorMessage).toBeDefined()
  })

  it("shows API error message", async () => {
    const fieldErrorMessage = "username already in use"
    server.use(
      rest.post("/api/v2/users", async (req, res, ctx) => {
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
    await renderCreateUserPage()
    await fillForm({})
    const errorMessage = await screen.findByText(fieldErrorMessage)
    expect(errorMessage).toBeDefined()
  })

  it("shows success notification and redirects to users page", async () => {
    await renderCreateUserPage()
    await fillForm({})
    const successMessage = screen.findByText(
      CreateUserLanguage.createUserSuccess,
    )
    expect(successMessage).toBeDefined()
  })
})
