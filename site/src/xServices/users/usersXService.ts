import { getPaginationData } from "components/PaginationWidget/utils"
import {
  PaginationContext,
  paginationMachine,
  PaginationMachineRef,
} from "xServices/pagination/paginationXService"
import { assign, createMachine, send, spawn } from "xstate"
import * as API from "../../api/api"
import { getErrorMessage } from "../../api/errors"
import * as TypesGen from "../../api/typesGenerated"
import {
  displayError,
  displaySuccess,
} from "../../components/GlobalSnackbar/utils"
import { queryToFilter } from "../../util/filters"
import { generateRandomString } from "../../util/random"

const usersPaginationId = "usersPagination"

export const Language = {
  getUsersError: "Error getting users.",
  suspendUserSuccess: "Successfully suspended the user.",
  suspendUserError: "Error suspending user.",
  deleteUserSuccess: "Successfully deleted the user.",
  deleteUserError: "Error deleting user.",
  activateUserSuccess: "Successfully activated the user.",
  activateUserError: "Error activating user.",
  resetUserPasswordSuccess: "Successfully updated the user password.",
  resetUserPasswordError: "Error on resetting the user password.",
  updateUserRolesSuccess: "Successfully updated the user roles.",
  updateUserRolesError: "Error on updating the user roles.",
}

export interface UsersContext {
  // Get users
  users?: TypesGen.User[]
  filter: string
  getUsersError?: Error | unknown
  // Suspend user
  userIdToSuspend?: TypesGen.User["id"]
  usernameToSuspend?: TypesGen.User["username"]
  suspendUserError?: Error | unknown
  // Delete user
  userIdToDelete?: TypesGen.User["id"]
  usernameToDelete?: TypesGen.User["username"]
  deleteUserError?: Error | unknown
  // Activate user
  userIdToActivate?: TypesGen.User["id"]
  usernameToActivate?: TypesGen.User["username"]
  activateUserError?: Error | unknown
  // Reset user password
  userIdToResetPassword?: TypesGen.User["id"]
  resetUserPasswordError?: Error | unknown
  newUserPassword?: string
  // Update user roles
  userIdToUpdateRoles?: TypesGen.User["id"]
  updateUserRolesError?: Error | unknown
  paginationContext: PaginationContext
  paginationRef: PaginationMachineRef
}

export type UsersEvent =
  | { type: "GET_USERS"; query?: string }
  // Suspend events
  | {
      type: "SUSPEND_USER"
      userId: TypesGen.User["id"]
      username: TypesGen.User["username"]
    }
  | { type: "CONFIRM_USER_SUSPENSION" }
  | { type: "CANCEL_USER_SUSPENSION" }
  // Delete events
  | {
      type: "DELETE_USER"
      userId: TypesGen.User["id"]
      username: TypesGen.User["username"]
    }
  | { type: "CONFIRM_USER_DELETE" }
  | { type: "CANCEL_USER_DELETE" }
  // Activate events
  | {
      type: "ACTIVATE_USER"
      userId: TypesGen.User["id"]
      username: TypesGen.User["username"]
    }
  | { type: "CONFIRM_USER_ACTIVATION" }
  | { type: "CANCEL_USER_ACTIVATION" }
  // Reset password events
  | { type: "RESET_USER_PASSWORD"; userId: TypesGen.User["id"] }
  | { type: "CONFIRM_USER_PASSWORD_RESET" }
  | { type: "CANCEL_USER_PASSWORD_RESET" }
  // Update roles events
  | {
      type: "UPDATE_USER_ROLES"
      userId: TypesGen.User["id"]
      roles: TypesGen.Role["name"][]
    }
  // Filter
  | { type: "UPDATE_FILTER"; query: string }
  // Pagination
  | { type: "UPDATE_PAGE"; page: string }

export const usersMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QFdZgE6wMoBcCGOYAdLPujgJYB2UACnlNQRQPZUDEA2gAwC6ioAA4tYFSmwEgAHogCMADiLcA7AFYVAFgCcW5bNkaAbACZDAGhABPOVsNEAzN1XGDhw92PduW+wF9fFqgY2PiERDA4lDQAqmiY7BBsxNQAbiwA1sQRscE8-EggwqLiVJIyCPKyRHryyk4m9lrGzfYW1ggaGqpERvaq9soasqrKfgEgQZi4BFlgkdRQOfEY6CzoRIIANgQAZmsAtuFzS7B5kkVirKUF5QC0ssrGRLV1rs2VyrptiJ3K1cbyHQaUyqSoDVT+QJxEIzIgUCCbMDsLDRLC0ACiADkACIAfVR6IASmcChcSmVEMZOkRDPpgYZ5J4NNx5PZWlY5A8qvZjH15Nx7IZVFp5KoNJCJtDpmF4Yj2Nj0QAZdEAFXR+KwRJJQhElwkN0phi0RAMqncjIZo0Z3wqtSIOgZGg+3HNsglkxhMoRSIAggBhFUASQAaj61RqtXxzrryQaEKZ7Pb7KLBQCTLJ3Kobe5E2adINjE1DE7jO6paFkt72IT0ZqVRHCbjaD6sFgAOoAeUJ2O1hRjVwp8dkxq6A2UFuTyhMWY5CFcdlkhbB8n5vNBZeC0srcuitGxYfVBMbhI7yqwvbJA7jtwBfwF3lG-WHPhtjO4NIZ7lk9Q0fI3UwrOEq13fdw2bABxdEL37fVQDuHk7BUTQnEcAEdGzLoaQtdQ+h8Ywp3-T1tyRECD1xAAxQNFTVYko1JGDrjgxB7m6LQjABfCtBUYduFkV9h2qAZv3w5wuIhcYPS3IgAGM2B2Ch0H2JYsFQQQwCoUQ2HYP0O0xSjCQAWQbXEUTRLEsEDXToOKK8mNtRMCyFXpPjcLQbX0FcTU+EtnFwhlCKk2SqHkxTlNU9TNI4P0fUxP0lWM0yMUxCyrLonUbNg6REFBbpmU+LiHipbhOnc-D3xXUURVpWlioCwCgpCpS4mxMBERKbTdP0oyj1xBVlTVay9UYrKKh8e1vHkbRgRUBobS0fQelFRc9F-Nj5EMOrYQahSmowFq2qubSYrixVjL61UoLSvsMuG8paSeYZ7AzNlDHHN65rcGliv5AVPjNExNrCbbQriH1pMoFJmC0nS9MDQzjP9INQyDVL8nSobBxeD8BnsTops6NzZ0MXHFrZKk2NUfQNok8strknaljBiGoai474p6xGQzDSzMUG2M7OFVjuMXQVeNUSmbTqRNlt-NwnuUR5xRpzdANgcKqAgBYlgSJI4SoNJMhIdWICWPnbJG4ZFxNcX+XHbxhzUOa6i8o1vBFdRaTdZWANhNXYDUjWtbidgVjWDZthwPZFKN-31JNuIzcy8oPPfEYXEm3R8NGPjZ1kRwNCUYU8-w2Xv3EqEVdhCBWrmIOMB1qhkn1jJiGrtqwFNq7LyTuRKeNAxmS0UENEeXjJZGapmRGEfVrTwHW5rqJFmD0P1i2XYDiINu5g7hOu4Ywd9CH6oWVFV4uItdzeONMU6TZWpFxH+eiDwcGKEhpftcSRu9YN4hX+ZoQTuaNroYzjJbRMPgeQujUOob85hZxmlTrSEYLpTAKDNM-AB79mAxBXugVYa8I5R0ONgj+u8MCJ1upyWwk9vwsmTMVAw48C7aAZFyMUU59DP2BrtdA9BYCwAAO5rAgISOAcwOqw3hj1ZsrZOzdlxDWOsVDMZimqPAgwxhKbixFO5HCX1ibaABJTV6ygeH0xBhgARwjRHiLQDgI6sV2aakbHI9sXY8TKNVKouMad7T9FsMTI0Csnr6LtAYUWahFxMnMd7IiRB0ASPmHg6xeBBEiPQBABuTc-6JOSUsGxmSIC+LsnndRecmjqD0KMfC7lxwLlMLofot5RTUwrj7MISSHGfziEU0RIcCFh3XpHTe3Tjh9PSbYrJpSLYPGNCoRwoJ3iihXO5IUTwRS6BqMWAYwJn7IEEBAXBy8MCEhYIiWAOTf4tyIIc45QC4jnMubMu4VoTTFSFG+SpGgPp-CnFA4SzhtDl0lJXMI9yTlLGeXAQZhDw4b2jpCx5ZyLlwFecxL8Dghi6FegoPogp+LGlGHoDwahCxOH8OMKgLBq7wAKJJVWZAl70EYFQFm0YbqDluM4E+nxPgsmLCKXkr5uiFktDob8zJXpKw6QkiIvTgicrAXZW47geiSqGHigEi4ZztBBEQfobIjWeBiVoZ+sowDKv5hbHkzwmiDB5M0NwYpfmzjUAXZwZ9OjfjYk9CxwUGZxBUrHDS5tu7UIQKCRQXEhgjzluUnO7Qj4F0qBgqBeF2lgs6cQXhSx9q10yhGwcJgnhGlpI8IeT5xZzQWk6dQ6h+QPHFmMOVgVLF8KZjgm1xa4zVUnkaZkuMvBDD1YgIxpMeRTUphmZ+fsA6a1Sega15tk4ZkUEXV2TgxTCnkO5YYfwnoKHFVSSccS22AW3oq5d9EuXgIUFUHQvFTCvAMHo2cK4C4eCEn0dwXQHhYLfh-OuN70Y2rXWNbRDribzSNUm8dngTTfkBLyFB8aA2NUKVM4p9i5grp7lG+ahq1AijzsCcl8G5wGPcEYpoS0zHP3GSk05-DsOiPw5GjyC5nBeFZOaDwY65xsI-GoWoDb1pqAOUcqFTy0X0rA6u5inR3xrlsCyOoAohhzSMPaGpr03C8TZPPDj3KGkfKMMswzbEbS3AcgKRcSEnArkGPIKlvggA */
  createMachine(
    {
      tsTypes: {} as import("./usersXService.typegen").Typegen0,
      schema: {
        context: {} as UsersContext,
        events: {} as UsersEvent,
        services: {} as {
          getUsers: {
            data: TypesGen.User[]
          }
          createUser: {
            data: TypesGen.User
          }
          suspendUser: {
            data: TypesGen.User
          }
          deleteUser: {
            data: undefined
          }
          activateUser: {
            data: TypesGen.User
          }
          updateUserPassword: {
            data: undefined
          }
          updateUserRoles: {
            data: TypesGen.User
          }
        },
      },
      predictableActionArguments: true,
      id: "usersState",
      initial: "startingPagination",
      states: {
        startingPagination: {
          entry: "assignPaginationRef",
          always: {
            target: "gettingUsers",
          },
        },
        gettingUsers: {
          entry: "clearGetUsersError",
          invoke: {
            src: "getUsers",
            id: "getUsers",
            onDone: [
              {
                target: "idle",
                actions: "assignUsers",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: [
                  "clearUsers",
                  "assignGetUsersError",
                  "displayGetUsersErrorMessage",
                ],
              },
            ],
          },
          tags: "loading",
        },
        idle: {
          entry: "clearSelectedUser",
          on: {
            SUSPEND_USER: {
              target: "confirmUserSuspension",
              actions: "assignUserToSuspend",
            },
            DELETE_USER: {
              target: "confirmUserDeletion",
              actions: "assignUserToDelete",
            },
            ACTIVATE_USER: {
              target: "confirmUserActivation",
              actions: "assignUserToActivate",
            },
            RESET_USER_PASSWORD: {
              target: "confirmUserPasswordReset",
              actions: [
                "assignUserIdToResetPassword",
                "generateRandomPassword",
              ],
            },
            UPDATE_USER_ROLES: {
              target: "updatingUserRoles",
              actions: "assignUserIdToUpdateRoles",
            },
            UPDATE_PAGE: {
              target: "gettingUsers",
              actions: "updateURL",
            },
            UPDATE_FILTER: {
              actions: ["assignFilter", "sendResetPage"],
            },
          },
        },
        confirmUserSuspension: {
          on: {
            CONFIRM_USER_SUSPENSION: {
              target: "suspendingUser",
            },
            CANCEL_USER_SUSPENSION: {
              target: "idle",
            },
          },
        },
        confirmUserDeletion: {
          on: {
            CONFIRM_USER_DELETE: {
              target: "deletingUser",
            },
            CANCEL_USER_DELETE: {
              target: "idle",
            },
          },
        },
        confirmUserActivation: {
          on: {
            CONFIRM_USER_ACTIVATION: {
              target: "activatingUser",
            },
            CANCEL_USER_ACTIVATION: {
              target: "idle",
            },
          },
        },
        suspendingUser: {
          entry: "clearSuspendUserError",
          invoke: {
            src: "suspendUser",
            id: "suspendUser",
            onDone: [
              {
                target: "gettingUsers",
                actions: "displaySuspendSuccess",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: [
                  "assignSuspendUserError",
                  "displaySuspendedErrorMessage",
                ],
              },
            ],
          },
        },
        deletingUser: {
          entry: "clearDeleteUserError",
          invoke: {
            src: "deleteUser",
            id: "deleteUser",
            onDone: [
              {
                target: "gettingUsers",
                actions: "displayDeleteSuccess",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: ["assignDeleteUserError", "displayDeleteErrorMessage"],
              },
            ],
          },
        },
        activatingUser: {
          entry: "clearActivateUserError",
          invoke: {
            src: "activateUser",
            id: "activateUser",
            onDone: [
              {
                target: "gettingUsers",
                actions: "displayActivateSuccess",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: [
                  "assignActivateUserError",
                  "displayActivatedErrorMessage",
                ],
              },
            ],
          },
        },
        confirmUserPasswordReset: {
          on: {
            CONFIRM_USER_PASSWORD_RESET: {
              target: "resettingUserPassword",
            },
            CANCEL_USER_PASSWORD_RESET: {
              target: "idle",
            },
          },
        },
        resettingUserPassword: {
          entry: "clearResetUserPasswordError",
          invoke: {
            src: "resetUserPassword",
            id: "resetUserPassword",
            onDone: [
              {
                target: "idle",
                actions: "displayResetPasswordSuccess",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: [
                  "assignResetUserPasswordError",
                  "displayResetPasswordErrorMessage",
                ],
              },
            ],
          },
        },
        updatingUserRoles: {
          entry: "clearUpdateUserRolesError",
          invoke: {
            src: "updateUserRoles",
            id: "updateUserRoles",
            onDone: [
              {
                target: "idle",
                actions: "updateUserRolesInTheList",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: [
                  "assignUpdateRolesError",
                  "displayUpdateRolesErrorMessage",
                ],
              },
            ],
          },
        },
      },
    },
    {
      services: {
        // Passing API.getUsers directly does not invoke the function properly
        // when it is mocked. This happen in the UsersPage tests inside of the
        // "shows a success message and refresh the page" test case.
        getUsers: (context) => {
          const { offset, limit } = getPaginationData(context.paginationRef)
          return API.getUsers({
            ...queryToFilter(context.filter),
            offset,
            limit,
          })
        },
        suspendUser: (context) => {
          if (!context.userIdToSuspend) {
            throw new Error("userIdToSuspend is undefined")
          }

          return API.suspendUser(context.userIdToSuspend)
        },
        deleteUser: (context) => {
          if (!context.userIdToDelete) {
            throw new Error("userIdToDelete is undefined")
          }
          return API.deleteUser(context.userIdToDelete)
        },
        activateUser: (context) => {
          if (!context.userIdToActivate) {
            throw new Error("userIdToActivate is undefined")
          }

          return API.activateUser(context.userIdToActivate)
        },
        resetUserPassword: (context) => {
          if (!context.userIdToResetPassword) {
            throw new Error("userIdToResetPassword is undefined")
          }

          if (!context.newUserPassword) {
            throw new Error("newUserPassword not generated")
          }

          return API.updateUserPassword(context.userIdToResetPassword, {
            password: context.newUserPassword,
            old_password: "",
          })
        },
        updateUserRoles: (context, event) => {
          if (!context.userIdToUpdateRoles) {
            throw new Error("userIdToUpdateRoles is undefined")
          }

          return API.updateUserRoles(event.roles, context.userIdToUpdateRoles)
        },
      },

      actions: {
        clearSelectedUser: assign({
          userIdToSuspend: (_) => undefined,
          usernameToSuspend: (_) => undefined,
          userIdToDelete: (_) => undefined,
          usernameToDelete: (_) => undefined,
          userIdToActivate: (_) => undefined,
          usernameToActivate: (_) => undefined,
          userIdToResetPassword: (_) => undefined,
          userIdToUpdateRoles: (_) => undefined,
        }),
        assignUsers: assign({
          users: (_, event) => event.data,
        }),
        assignFilter: assign({
          filter: (_, event) => event.query,
        }),
        assignGetUsersError: assign({
          getUsersError: (_, event) => event.data,
        }),
        assignUserToSuspend: assign({
          userIdToSuspend: (_, event) => event.userId,
          usernameToSuspend: (_, event) => event.username,
        }),
        assignUserToDelete: assign({
          userIdToDelete: (_, event) => event.userId,
          usernameToDelete: (_, event) => event.username,
        }),
        assignUserToActivate: assign({
          userIdToActivate: (_, event) => event.userId,
          usernameToActivate: (_, event) => event.username,
        }),
        assignUserIdToResetPassword: assign({
          userIdToResetPassword: (_, event) => event.userId,
        }),
        assignUserIdToUpdateRoles: assign({
          userIdToUpdateRoles: (_, event) => event.userId,
        }),
        clearGetUsersError: assign((context: UsersContext) => ({
          ...context,
          getUsersError: undefined,
        })),
        assignSuspendUserError: assign({
          suspendUserError: (_, event) => event.data,
        }),
        assignDeleteUserError: assign({
          deleteUserError: (_, event) => event.data,
        }),
        assignActivateUserError: assign({
          activateUserError: (_, event) => event.data,
        }),
        assignResetUserPasswordError: assign({
          resetUserPasswordError: (_, event) => event.data,
        }),
        assignUpdateRolesError: assign({
          updateUserRolesError: (_, event) => event.data,
        }),
        clearUsers: assign((context: UsersContext) => ({
          ...context,
          users: undefined,
        })),
        clearSuspendUserError: assign({
          suspendUserError: (_) => undefined,
        }),
        clearDeleteUserError: assign({
          deleteUserError: (_) => undefined,
        }),
        clearActivateUserError: assign({
          activateUserError: (_) => undefined,
        }),
        clearResetUserPasswordError: assign({
          resetUserPasswordError: (_) => undefined,
        }),
        clearUpdateUserRolesError: assign({
          updateUserRolesError: (_) => undefined,
        }),
        displayGetUsersErrorMessage: (context) => {
          const message = getErrorMessage(
            context.getUsersError,
            Language.getUsersError,
          )
          displayError(message)
        },
        displaySuspendSuccess: () => {
          displaySuccess(Language.suspendUserSuccess)
        },
        displaySuspendedErrorMessage: (context) => {
          const message = getErrorMessage(
            context.suspendUserError,
            Language.suspendUserError,
          )
          displayError(message)
        },
        displayDeleteSuccess: () => {
          displaySuccess(Language.deleteUserSuccess)
        },
        displayDeleteErrorMessage: (context) => {
          const message = getErrorMessage(
            context.deleteUserError,
            Language.deleteUserError,
          )
          displayError(message)
        },
        displayActivateSuccess: () => {
          displaySuccess(Language.activateUserSuccess)
        },
        displayActivatedErrorMessage: (context) => {
          const message = getErrorMessage(
            context.activateUserError,
            Language.activateUserError,
          )
          displayError(message)
        },
        displayResetPasswordSuccess: () => {
          displaySuccess(Language.resetUserPasswordSuccess)
        },
        displayResetPasswordErrorMessage: (context) => {
          const message = getErrorMessage(
            context.resetUserPasswordError,
            Language.resetUserPasswordError,
          )
          displayError(message)
        },
        displayUpdateRolesErrorMessage: (context) => {
          const message = getErrorMessage(
            context.updateUserRolesError,
            Language.updateUserRolesError,
          )
          displayError(message)
        },
        generateRandomPassword: assign({
          newUserPassword: (_) => generateRandomString(12),
        }),
        updateUserRolesInTheList: assign({
          users: ({ users }, event) => {
            if (!users) {
              return users
            }

            return users.map((u) => {
              return u.id === event.data.id ? event.data : u
            })
          },
        }),
        assignPaginationRef: assign({
          paginationRef: (context) =>
            spawn(
              paginationMachine.withContext(context.paginationContext),
              usersPaginationId,
            ),
        }),
        sendResetPage: send({ type: "RESET_PAGE" }, { to: usersPaginationId }),
      },
    },
  )
