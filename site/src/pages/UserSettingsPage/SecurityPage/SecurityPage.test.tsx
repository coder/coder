import { fireEvent, screen, waitFor } from "@testing-library/react"
import * as API from "../../../api/api"
import * as SecurityForm from "../../../components/SettingsSecurityForm/SettingsSecurityForm"
import {
  renderWithAuth,
  waitForLoaderToBeRemoved,
} from "../../../testHelpers/renderHelpers"
import { SecurityPage } from "./SecurityPage"
import i18next from "i18next"
import {
  MockAuthMethodsWithPasswordType,
  mockApiError,
} from "testHelpers/entities"

const { t } = i18next

const renderPage = async () => {
  const utils = renderWithAuth(<SecurityPage />)
  await waitForLoaderToBeRemoved()
  return utils
}

const newSecurityFormValues = {
  old_password: "password1",
  password: "password2",
  confirm_password: "password2",
}

const fillAndSubmitSecurityForm = () => {
  fireEvent.change(screen.getByLabelText("Old Password"), {
    target: { value: newSecurityFormValues.old_password },
  })
  fireEvent.change(screen.getByLabelText("New Password"), {
    target: { value: newSecurityFormValues.password },
  })
  fireEvent.change(screen.getByLabelText("Confirm Password"), {
    target: { value: newSecurityFormValues.confirm_password },
  })
  fireEvent.click(screen.getByText(SecurityForm.Language.updatePassword))
}

beforeEach(() => {
  jest
    .spyOn(API, "getAuthMethods")
    .mockResolvedValue(MockAuthMethodsWithPasswordType)
})

test("update password successfully", async () => {
  jest
    .spyOn(API, "updateUserPassword")
    .mockImplementationOnce((_userId, _data) => Promise.resolve(undefined))
  const { user } = await renderPage()
  fillAndSubmitSecurityForm()

  const expectedMessage = t("securityUpdateSuccessMessage", {
    ns: "userSettingsPage",
  })
  const successMessage = await screen.findByText(expectedMessage)
  expect(successMessage).toBeDefined()
  expect(API.updateUserPassword).toBeCalledTimes(1)
  expect(API.updateUserPassword).toBeCalledWith(user.id, newSecurityFormValues)

  await waitFor(() => expect(window.location).toBeAt("/"))
})

test("update password with incorrect old password", async () => {
  jest.spyOn(API, "updateUserPassword").mockRejectedValueOnce(
    mockApiError({
      message: "Incorrect password.",
      validations: [{ detail: "Incorrect password.", field: "old_password" }],
    }),
  )

  const { user } = await renderPage()
  fillAndSubmitSecurityForm()

  const errorMessage = await screen.findAllByText("Incorrect password.")
  expect(errorMessage).toBeDefined()
  expect(errorMessage).toHaveLength(2)
  expect(API.updateUserPassword).toBeCalledTimes(1)
  expect(API.updateUserPassword).toBeCalledWith(user.id, newSecurityFormValues)
})

test("update password with invalid password", async () => {
  jest.spyOn(API, "updateUserPassword").mockRejectedValueOnce(
    mockApiError({
      message: "Invalid password.",
      validations: [{ detail: "Invalid password.", field: "password" }],
    }),
  )

  const { user } = await renderPage()
  fillAndSubmitSecurityForm()

  const errorMessage = await screen.findAllByText("Invalid password.")
  expect(errorMessage).toBeDefined()
  expect(errorMessage).toHaveLength(2)
  expect(API.updateUserPassword).toBeCalledTimes(1)
  expect(API.updateUserPassword).toBeCalledWith(user.id, newSecurityFormValues)
})

test("update password when submit returns an unknown error", async () => {
  jest.spyOn(API, "updateUserPassword").mockRejectedValueOnce({
    data: "unknown error",
  })

  const { user } = await renderPage()
  fillAndSubmitSecurityForm()

  const errorText = t("warningsAndErrors.somethingWentWrong", {
    ns: "common",
  })
  const errorMessage = await screen.findByText(errorText)
  expect(errorMessage).toBeDefined()
  expect(API.updateUserPassword).toBeCalledTimes(1)
  expect(API.updateUserPassword).toBeCalledWith(user.id, newSecurityFormValues)
})
