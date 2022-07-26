import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import { rest } from "msw"
import * as API from "../../api/api"
import { Role } from "../../api/typesGenerated"
import { GlobalSnackbar } from "../../components/GlobalSnackbar/GlobalSnackbar"
import { Language as ResetPasswordDialogLanguage } from "../../components/ResetPasswordDialog/ResetPasswordDialog"
import { Language as RoleSelectLanguage } from "../../components/RoleSelect/RoleSelect"
import { Language as UsersTableBodyLanguage } from "../../components/UsersTable/UsersTableBody"
import {
  MockAuditorRole,
  MockUser,
  MockUser2,
  render,
  SuspendedMockUser,
} from "../../testHelpers/renderHelpers"
import { server } from "../../testHelpers/server"
import { permissionsToCheck } from "../../xServices/auth/authXService"
import { Language as usersXServiceLanguage } from "../../xServices/users/usersXService"
import { Language as UsersPageLanguage, UsersPage } from "./UsersPage"
import { Language as UsersViewLanguage } from "./UsersPageView"

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
  const suspendButton = within(menu).getByText(UsersTableBodyLanguage.suspendMenuItem)
  fireEvent.click(suspendButton)

  // Check if the confirm message is displayed
  const confirmDialog = screen.getByRole("dialog")
  expect(confirmDialog).toHaveTextContent(
    `${UsersPageLanguage.suspendDialogMessagePrefix} ${MockUser.username}?`,
  )

  // Setup spies to check the actions after
  setupActionSpies()

  // Click on the "Confirm" button
  const confirmButton = within(confirmDialog).getByText(UsersPageLanguage.suspendDialogAction)
  fireEvent.click(confirmButton)
}

const activateUser = async (setupActionSpies: () => void) => {
  // Get the first user in the table
  const users = await screen.findAllByText(/.*@coder.com/)
  const firstUserRow = users[2].closest("tr")
  if (!firstUserRow) {
    throw new Error("Error on get the first user row")
  }

  // Click on the "more" button to display the "Activate" option
  const moreButton = within(firstUserRow).getByLabelText("more")
  fireEvent.click(moreButton)
  const menu = screen.getByRole("menu")
  const activateButton = within(menu).getByText(UsersTableBodyLanguage.activateMenuItem)
  fireEvent.click(activateButton)

  // Check if the confirm message is displayed
  const confirmDialog = screen.getByRole("dialog")
  expect(confirmDialog).toHaveTextContent(
    `${UsersPageLanguage.activateDialogMessagePrefix} ${SuspendedMockUser.username}?`,
  )

  // Setup spies to check the actions after
  setupActionSpies()

  // Click on the "Confirm" button
  const confirmButton = within(confirmDialog).getByText(UsersPageLanguage.activateDialogAction)
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
  const resetPasswordButton = within(menu).getByText(UsersTableBodyLanguage.resetPasswordMenuItem)
  fireEvent.click(resetPasswordButton)

  // Check if the confirm message is displayed
  const confirmDialog = screen.getByRole("dialog")
  expect(confirmDialog).toHaveTextContent(
    `You will need to send ${MockUser.username} the following password:`,
  )

  // Setup spies to check the actions after
  setupActionSpies()

  // Click on the "Confirm" button
  const confirmButton = within(confirmDialog).getByRole("button", {
    name: ResetPasswordDialogLanguage.confirmText,
  })
  fireEvent.click(confirmButton)
}

const updateUserRole = async (setupActionSpies: () => void, role: Role) => {
  // Get the first user in the table
  const users = await screen.findAllByText(/.*@coder.com/)
  const firstUserRow = users[0].closest("tr")
  if (!firstUserRow) {
    throw new Error("Error on get the first user row")
  }

  // Click on the "roles" menu to display the role options
  const rolesLabel = within(firstUserRow).getByLabelText(RoleSelectLanguage.label)
  const rolesMenuTrigger = within(rolesLabel).getByRole("button")
  // For MUI v4, the Select was changed to open on mouseDown instead of click
  // https://github.com/mui-org/material-ui/pull/17978
  fireEvent.mouseDown(rolesMenuTrigger)

  // Setup spies to check the actions after
  setupActionSpies()

  // Click on the role option
  const listBox = screen.getByRole("listbox")
  const auditorOption = within(listBox).getByRole("option", { name: role.display_name })
  fireEvent.click(auditorOption)

  return {
    rolesMenuTrigger,
  }
}

describe("Users Page", () => {
  it("shows users", async () => {
    render(<UsersPage />)
    const users = await screen.findAllByText(/.*@coder.com/)
    expect(users.length).toEqual(3)
  })

  it("shows 'Create user' button to an authorized user", () => {
    render(<UsersPage />)
    const createUserButton = screen.queryByText(UsersViewLanguage.createButton)
    expect(createUserButton).toBeDefined()
  })

  it("does not show 'Create user' button to unauthorized user", () => {
    server.use(
      rest.post("/api/v2/users/:userId/authorization", async (req, res, ctx) => {
        const permissions = Object.keys(permissionsToCheck)
        const response = permissions.reduce((obj, permission) => {
          return {
            ...obj,
            [permission]: true,
            createUser: false,
          }
        }, {})

        return res(ctx.status(200), ctx.json(response))
      }),
    )
    render(<UsersPage />)
    const createUserButton = screen.queryByText(UsersViewLanguage.createButton)
    expect(createUserButton).toBeNull()
  })

  describe("suspend user", () => {
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

  describe("activate user", () => {
    describe("when user is successfully activated", () => {
      it("shows a success message and refreshes the page", async () => {
        render(
          <>
            <UsersPage />
            <GlobalSnackbar />
          </>,
        )

        await activateUser(() => {
          jest.spyOn(API, "activateUser").mockResolvedValueOnce(SuspendedMockUser)
          jest
            .spyOn(API, "getUsers")
            .mockImplementationOnce(() => Promise.resolve([MockUser, MockUser2, SuspendedMockUser]))
        })

        // Check if the success message is displayed
        await screen.findByText(usersXServiceLanguage.activateUserSuccess)

        // Check if the API was called correctly
        expect(API.activateUser).toBeCalledTimes(1)
        expect(API.activateUser).toBeCalledWith(SuspendedMockUser.id)
      })
    })
    describe("when activation fails", () => {
      it("shows an error message", async () => {
        render(
          <>
            <UsersPage />
            <GlobalSnackbar />
          </>,
        )

        await activateUser(() => {
          jest.spyOn(API, "activateUser").mockRejectedValueOnce({})
        })

        // Check if the error message is displayed
        await screen.findByText(usersXServiceLanguage.activateUserError)

        // Check if the API was called correctly
        expect(API.activateUser).toBeCalledTimes(1)
        expect(API.activateUser).toBeCalledWith(SuspendedMockUser.id)
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
        expect(API.updateUserPassword).toBeCalledWith(MockUser.id, {
          password: expect.any(String),
          old_password: "",
        })
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
        expect(API.updateUserPassword).toBeCalledWith(MockUser.id, {
          password: expect.any(String),
          old_password: "",
        })
      })
    })
  })

  describe("Update user role", () => {
    describe("when it is success", () => {
      it("updates the roles", async () => {
        render(
          <>
            <UsersPage />
            <GlobalSnackbar />
          </>,
        )

        const { rolesMenuTrigger } = await updateUserRole(() => {
          jest.spyOn(API, "updateUserRoles").mockResolvedValueOnce({
            ...MockUser,
            roles: [...MockUser.roles, MockAuditorRole],
          })
        }, MockAuditorRole)

        // Check if the select text was updated with the Auditor role
        await waitFor(() => expect(rolesMenuTrigger).toHaveTextContent("Admin, Auditor"))

        // Check if the API was called correctly
        const currentRoles = MockUser.roles.map((r) => r.name)
        expect(API.updateUserRoles).toBeCalledTimes(1)
        expect(API.updateUserRoles).toBeCalledWith(
          [...currentRoles, MockAuditorRole.name],
          MockUser.id,
        )
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

        await updateUserRole(() => {
          jest.spyOn(API, "updateUserRoles").mockRejectedValueOnce({})
        }, MockAuditorRole)

        // Check if the error message is displayed
        await screen.findByText(usersXServiceLanguage.updateUserRolesError)

        // Check if the API was called correctly
        const currentRoles = MockUser.roles.map((r) => r.name)
        expect(API.updateUserRoles).toBeCalledTimes(1)
        expect(API.updateUserRoles).toBeCalledWith(
          [...currentRoles, MockAuditorRole.name],
          MockUser.id,
        )
      })
      it("shows an error from the backend", async () => {
        render(
          <>
            <UsersPage />
            <GlobalSnackbar />
          </>,
        )

        server.use(
          rest.put(`/api/v2/users/${MockUser.id}/roles`, (req, res, ctx) => {
            return res(ctx.status(401), ctx.json({ message: "message from the backend" }))
          }),
        )

        // eslint-disable-next-line @typescript-eslint/no-empty-function
        await updateUserRole(() => {}, MockAuditorRole)

        // Check if the error message is displayed
        await screen.findByText("message from the backend")
      })
    })
  })
})
