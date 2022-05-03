import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import React from "react"
import * as API from "../../api"
import { GlobalSnackbar } from "../../components/GlobalSnackbar/GlobalSnackbar"
import { Language as UsersTableLanguage } from "../../components/UsersTable/UsersTable"
import { MockUser, MockUser2, render } from "../../testHelpers"
import { Language as usersXServiceLanguage } from "../../xServices/users/usersXService"
import { Language as UsersPageLanguage, UsersPage } from "./UsersPage"

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
        expect(confirmDialog).toHaveTextContent("Do you want to suspend the user TestUser?")

        // Setup spies to check the actions after
        jest.spyOn(API, "suspendUser").mockResolvedValueOnce(MockUser)
        jest.spyOn(API, "getUsers").mockImplementationOnce(() => Promise.resolve([MockUser, MockUser2]))

        // Click on the "Confirm" button
        const confirmButton = within(confirmDialog).getByText(UsersPageLanguage.suspendDialogAction)
        fireEvent.click(confirmButton)

        // Check if the success message
        await screen.findByText(usersXServiceLanguage.suspendUserSuccess)

        // Check if the API was called correctly
        expect(API.suspendUser).toBeCalledTimes(1)
        expect(API.suspendUser).toBeCalledWith(MockUser.id)

        // Check if the users list was refetched
        await waitFor(() => expect(API.getUsers).toBeCalledTimes(1))
      })
    })
  })
})
