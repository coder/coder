import { fireEvent, screen, waitFor } from "@testing-library/react"
import React from "react"
import * as API from "../../api"
import * as AccountForm from "../../components/Preferences/AccountForm"
import { GlobalSnackbar } from "../../components/Snackbar/GlobalSnackbar"
import { renderWithAuth } from "../../test_helpers/render"
import * as AuthXService from "../../xServices/auth/authXService"
import { PreferencesAccountPage } from "./account"

describe("PreferencesAccountPage", () => {
  describe("when it is a success", () => {
    it("shows the success message", async () => {
      const newUserProfile = {
        name: "User",
        email: "user@coder.com",
        username: "user",
      }

      jest.spyOn(API, "updateProfile").mockImplementationOnce((userId, data) =>
        Promise.resolve({
          id: userId,
          ...data,
          created_at: new Date().toString(),
        }),
      )

      const { user } = renderWithAuth(
        <>
          <PreferencesAccountPage />
          <GlobalSnackbar />
        </>,
      )

      // Wait for the form to load
      await waitFor(() => screen.findByLabelText("Name"), { timeout: 50000 })
      fireEvent.change(screen.getByLabelText("Name"), { target: { value: newUserProfile.name } })
      fireEvent.change(screen.getByLabelText("Email"), { target: { value: newUserProfile.email } })
      fireEvent.change(screen.getByLabelText("Username"), { target: { value: newUserProfile.username } })
      fireEvent.click(screen.getByText(AccountForm.Language.updatePreferences))

      const successMessage = await screen.findByText(AuthXService.Language.successProfileUpdate)
      expect(successMessage).toBeDefined()
      expect(API.updateProfile).toBeCalledTimes(1)
      expect(API.updateProfile).toBeCalledWith(user.id, newUserProfile)
    })
  })
})
