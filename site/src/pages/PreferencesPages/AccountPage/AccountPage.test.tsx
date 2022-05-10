import { fireEvent, screen, waitFor } from "@testing-library/react"
import React from "react"
import * as API from "../../../api"
import { GlobalSnackbar } from "../../../components/GlobalSnackbar/GlobalSnackbar"
import * as AccountForm from "../../../components/PreferencesAccountForm/PreferencesAccountForm"
import { renderWithAuth } from "../../../testHelpers"
import * as AuthXService from "../../../xServices/auth/authXService"
import { AccountPage, Language } from "./AccountPage"

const renderPage = () => {
  return renderWithAuth(
    <>
      <AccountPage />
      <GlobalSnackbar />
    </>,
  )
}

const newData = {
  email: "user@coder.com",
  username: "user",
}

const fillAndSubmitForm = async () => {
  await waitFor(() => screen.findByLabelText("Email"))
  fireEvent.change(screen.getByLabelText("Email"), { target: { value: newData.email } })
  fireEvent.change(screen.getByLabelText("Username"), { target: { value: newData.username } })
  fireEvent.click(screen.getByText(AccountForm.Language.updatePreferences))
}

describe("AccountPage", () => {
  describe("when it is a success", () => {
    it("shows the success message", async () => {
      jest.spyOn(API, "updateProfile").mockImplementationOnce((userId, data) =>
        Promise.resolve({
          id: userId,
          created_at: new Date().toString(),
          status: "active",
          organization_ids: ["123"],
          roles: [],
          ...data,
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
        response: {
          data: { message: "Invalid profile", errors: [{ detail: "Email is already in use", field: "email" }] },
        },
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
        response: {
          data: { message: "Invalid profile", errors: [{ detail: "Username is already in use", field: "username" }] },
        },
      })

      const { user } = renderPage()
      await fillAndSubmitForm()

      const errorMessage = await screen.findByText("Username is already in use")
      expect(errorMessage).toBeDefined()
      expect(API.updateProfile).toBeCalledTimes(1)
      expect(API.updateProfile).toBeCalledWith(user.id, newData)
    })
  })

  describe("when it is an unknown error", () => {
    it("shows a generic error message", async () => {
      jest.spyOn(API, "updateProfile").mockRejectedValueOnce({
        data: "unknown error",
      })

      const { user } = renderPage()
      await fillAndSubmitForm()

      const errorMessage = await screen.findByText(Language.unknownError)
      expect(errorMessage).toBeDefined()
      expect(API.updateProfile).toBeCalledTimes(1)
      expect(API.updateProfile).toBeCalledWith(user.id, newData)
    })
  })
})
