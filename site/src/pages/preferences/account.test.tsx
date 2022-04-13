import { fireEvent, screen, waitFor } from "@testing-library/react"
import React from "react"
import * as API from "../../api"
import * as AccountForm from "../../components/Preferences/AccountForm"
import { GlobalSnackbar } from "../../components/Snackbar/GlobalSnackbar"
import { renderWithAuth } from "../../test_helpers/render"
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

const fillTheForm = async () => {
  await waitFor(() => screen.findByLabelText("Name"))
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: newData.name } })
  fireEvent.change(screen.getByLabelText("Email"), { target: { value: newData.email } })
  fireEvent.change(screen.getByLabelText("Username"), { target: { value: newData.username } })
  fireEvent.click(screen.getByText(AccountForm.Language.updatePreferences))
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
      await fillTheForm()

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
        response: { data: { errors: [{ code: "exists", field: "email" }] } },
      })

      const { user } = renderPage()
      await fillTheForm()

      const errorMessage = await screen.findByText(API.Language.errorsByCode.exists)
      expect(errorMessage).toBeDefined()
      expect(API.updateProfile).toBeCalledTimes(1)
      expect(API.updateProfile).toBeCalledWith(user.id, newData)
    })
  })

  describe("when the username is already taken", () => {
    it("shows an error", async () => {
      jest.spyOn(API, "updateProfile").mockRejectedValueOnce({
        isAxiosError: true,
        response: { data: { errors: [{ code: "exists", field: "username" }] } },
      })

      const { user } = renderPage()
      await fillTheForm()

      const errorMessage = await screen.findByText(API.Language.errorsByCode.exists)
      expect(errorMessage).toBeDefined()
      expect(API.updateProfile).toBeCalledTimes(1)
      expect(API.updateProfile).toBeCalledWith(user.id, newData)
    })
  })
})
