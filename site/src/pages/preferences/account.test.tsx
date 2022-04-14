import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import React from "react"
import * as API from "../../api"
import * as AccountForm from "../../components/Preferences/AccountForm"
import { GlobalSnackbar } from "../../components/Snackbar/GlobalSnackbar"
import { renderWithAuth } from "../../test_helpers"
import * as AuthXService from "../../xServices/auth/authXService"
import { PreferencesAccountPage } from "./account"

const renderPage = () => {
  return renderWithAuth(
    <>
      <PreferencesAccountPage />
      <GlobalSnackbar />
    </>,
  )
}

const newData = {
  name: "User",
  email: "user@coder.com",
  username: "user",
}

const fillAndSubmitForm = async () => {
  await waitFor(() => screen.findByLabelText("Name"))
  await userEvent.type(screen.getByLabelText("Name"), newData.name)
  await userEvent.type(screen.getByLabelText("Email"), newData.email)
  await userEvent.type(screen.getByLabelText("Username"), newData.username)
  await userEvent.click(screen.getByText(AccountForm.Language.updatePreferences))
}

describe("PreferencesAccountPage", () => {
  afterEach(() => {
    jest.clearAllMocks()
  })

  describe("when it is a success", () => {
    it("shows the success message", async () => {
      jest.spyOn(API, "updateProfile").mockImplementationOnce((userId, data) =>
        Promise.resolve({
          id: userId,
          ...data,
          created_at: new Date().toString(),
        }),
      )
      const { user } = renderPage()
      await fillAndSubmitForm()

      const successMessage = await screen.findByText(AuthXService.Language.successProfileUpdate)
      expect(successMessage).toBeDefined()
      expect(API.updateProfile).toBeCalledTimes(1)
      expect(API.updateProfile).toBeCalledWith(user.id, newData)
    })
  })

  describe("when the email is already taken", () => {
    it("shows an error", async () => {
      jest.spyOn(API, "updateProfile").mockRejectedValueOnce({
        isAxiosError: true,
        response: { data: { errors: [{ detail: "Email is already in use", field: "email" }] } },
      })

      const { user } = renderPage()
      await fillAndSubmitForm()

      const errorMessage = await screen.findByText("Email is already in use")
      expect(errorMessage).toBeDefined()
      expect(API.updateProfile).toBeCalledTimes(1)
      expect(API.updateProfile).toBeCalledWith(user.id, newData)
    })
  })

  describe("when the username is already taken", () => {
    it("shows an error", async () => {
      jest.spyOn(API, "updateProfile").mockRejectedValueOnce({
        isAxiosError: true,
        response: { data: { errors: [{ detail: "Username is already in use", field: "username" }] } },
      })

      const { user } = renderPage()
      await fillAndSubmitForm()

      const errorMessage = await screen.findByText("Username is already in use")
      expect(errorMessage).toBeDefined()
      expect(API.updateProfile).toBeCalledTimes(1)
      expect(API.updateProfile).toBeCalledWith(user.id, newData)
    })
  })
})
