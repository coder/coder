import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import React from "react"
import * as API from "../../api"
import { GlobalSnackbar } from "../../components/GlobalSnackbar/GlobalSnackbar"
import { Language as ResetPasswordDialogLanguage } from "../../components/ResetPasswordDialog/ResetPasswordDialog"
import { Language as UsersTableLanguage } from "../../components/UsersTable/UsersTable"
import { MockUser, MockUser2, render } from "../../testHelpers"
import { Language as usersXServiceLanguage } from "../../xServices/users/usersXService"
import { Language as UsersPageLanguage, UsersPage } from "./UsersPage"

const suspendUser = async (setupActionSpies: () => void) => {
  // Get the first user in the table
  const users = await screen.findAllByText(/.*@coder.com/)
  const firstUserRow = users[0].closest("tr")
  if (!firstUserRow) {
    throw new Error("Error on get the first user row")
  }

  // Click on the "more" button to display the "Suspend" option
  const moreButton = within(firstUserRow).getByLabelText("more")
  fireEvent.click(moreButton)
  const menu = screen.getByRole("menu")
  const suspendButton = within(menu).getByText(UsersTableLanguage.suspendMenuItem)
  fireEvent.click(suspendButton)

  // Check if the confirm message is displayed
  const confirmDialog = screen.getByRole("dialog")
  expect(confirmDialog).toHaveTextContent(`${UsersPageLanguage.suspendDialogMessagePrefix} ${MockUser.username}?`)

  // Setup spies to check the actions after
  setupActionSpies()

  // Click on the "Confirm" button
  const confirmButton = within(confirmDialog).getByText(UsersPageLanguage.suspendDialogAction)
  fireEvent.click(confirmButton)
}

const resetUserPassword = async (setupActionSpies: () => void) => {
  // Get the first user in the table
  const users = await screen.findAllByText(/.*@coder.com/)
  const firstUserRow = users[0].closest("tr")
  if (!firstUserRow) {
    throw new Error("Error on get the first user row")
  }

  // Click on the "more" button to display the "Suspend" option
  const moreButton = within(firstUserRow).getByLabelText("more")
  fireEvent.click(moreButton)
  const menu = screen.getByRole("menu")
  const resetPasswordButton = within(menu).getByText(UsersTableLanguage.resetPasswordMenuItem)
  fireEvent.click(resetPasswordButton)

  // Check if the confirm message is displayed
  const confirmDialog = screen.getByRole("dialog")
  expect(confirmDialog).toHaveTextContent(`You will need to send ${MockUser.username} the following password:`)

  // Setup spies to check the actions after
  setupActionSpies()

  // Click on the "Confirm" button
  const confirmButton = within(confirmDialog).getByRole("button", { name: ResetPasswordDialogLanguage.confirmText })
  fireEvent.click(confirmButton)
}

describe("Users Page", () => {
  it("shows users", async () => {
    render(<UsersPage />)
    const users = await screen.findAllByText(/.*@coder.com/)
    expect(users.length).toEqual(2)
  })

  describe("suspend user", () => {
    describe("when it is success", () => {
      it("shows a success message and refresh the page", async () => {
        render(
          <>
            <UsersPage />
            <GlobalSnackbar />
          </>,
        )

        await suspendUser(() => {
          jest.spyOn(API, "suspendUser").mockResolvedValueOnce(MockUser)
          jest.spyOn(API, "getUsers").mockImplementationOnce(() => Promise.resolve([MockUser, MockUser2]))
        })

        // Check if the success message is displayed
        await screen.findByText(usersXServiceLanguage.suspendUserSuccess)

        // Check if the API was called correctly
        expect(API.suspendUser).toBeCalledTimes(1)
        expect(API.suspendUser).toBeCalledWith(MockUser.id)

        // Check if the users list was reload
        await waitFor(() => expect(API.getUsers).toBeCalledTimes(1))
      })
    })

    describe("when it fails", () => {
      it("shows an error message", async () => {
        render(
          <>
            <UsersPage />
            <GlobalSnackbar />
          </>,
        )

        await suspendUser(() => {
          jest.spyOn(API, "suspendUser").mockRejectedValueOnce({})
        })

        // Check if the error message is displayed
        await screen.findByText(usersXServiceLanguage.suspendUserError)

        // Check if the API was called correctly
        expect(API.suspendUser).toBeCalledTimes(1)
        expect(API.suspendUser).toBeCalledWith(MockUser.id)
      })
    })
  })

  describe("reset user password", () => {
    describe("when it is success", () => {
      it("shows a success message", async () => {
        render(
          <>
            <UsersPage />
            <GlobalSnackbar />
          </>,
        )

        await resetUserPassword(() => {
          jest.spyOn(API, "updateUserPassword").mockResolvedValueOnce(undefined)
        })

        // Check if the success message is displayed
        await screen.findByText(usersXServiceLanguage.resetUserPasswordSuccess)

        // Check if the API was called correctly
        expect(API.updateUserPassword).toBeCalledTimes(1)
        expect(API.updateUserPassword).toBeCalledWith(expect.any(String), MockUser.id)
      })
    })

    describe("when it fails", () => {
      it("shows an error message", async () => {
        render(
          <>
            <UsersPage />
            <GlobalSnackbar />
          </>,
        )

        await resetUserPassword(() => {
          jest.spyOn(API, "updateUserPassword").mockRejectedValueOnce({})
        })

        // Check if the error message is displayed
        await screen.findByText(usersXServiceLanguage.resetUserPasswordError)

        // Check if the API was called correctly
        expect(API.updateUserPassword).toBeCalledTimes(1)
        expect(API.updateUserPassword).toBeCalledWith(expect.any(String), MockUser.id)
      })
    })
  })
})
