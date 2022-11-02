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
  count: number
  getCountError: Error | unknown
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
  /** @xstate-layout N4IgpgJg5mDOIC5QFdZgE6wMoBcCGOYAdAMYD2yAdjkTDjgJaVQDCF1AxBGZcUwG5kA1sToBVNOjZUcAbQAMAXUSgADmVgNGPFSAAeiAIwBWQ-KIBmABwAWCwHYrAJhcA2YwE4ANCACeiZ0Miew8LBxsrCw8Pe1MAXzifVAxsfEJSdho6RmZpTgx0MnQiVQAbAgAzIoBbWjAcCQw8uSVddU1tSl0DBEMneSDXC0MrQ2G3KysffwRo4yJjYxtlh1c1m2NXBKTJVIJichkOMQAFABEAQQAVAFEAfQAxAEkAGVuAJQVlJBB2rQYdD8en1DB4iE5NvYbCMokNvH5EK4NkRbI5XIYbE5DGNDPZtiBkphcPsiITYERYPh0DkoCc8FAmAQAZQOF82hp-oDQD0ALRQ8zyEIWYz2dwhJw2VzTRGOIiGdyjJwWSXyZV4xIE3bE9Jkur0JhQRqYLg8PiUQQiPVG2Bsn5-TrdRA84wuYLyVxWexRDy2Vz2ezShCRGwLRZ+0VOWzGT34sna4i67IG60cApFErlHBVdC1cS7W1qDkOoFOxxOSxK7HySKReSmQNWd0LEJ9L1OWLGeQeWNatIJ3ZEBgQUpgDhYMRYE43AByZzuE5un1adqLzMdCB5SNcKOhNdcddiHsDGJCKOjHjGStsZicPZS8dJA6HI44ZxuLxut3nWEXBd+q65fQnSReZrEcKwfSsJZNgsY9hkGNUIklRZBQsO8iT7R8UkHYdRwuFgrieAA1a57gXJdvkLDo1xLDcQKId1llcGIoUiJx4RmbEbDBP1QXFGIIVFdC9h1J9cI4d4bh-K5v0XO4TguLAsAAdQAeXeM4-3tGjuWAmx7Dlex2MlCEPCGGxjyGbcbEFDxBRskx7FxYSH11Z9R1OS4v3Iu53lUj8sC0gCulonkfW3eUsRiDEfWMaxjxGAy4rDD1wwxLYNTjTC3PEzzSPki4AHEbiC6jAN5DxJTlDEIOhD15TsQMPE7Bi1n9Vs7MbdUdnvbKB3ISgKgYHMjSwVBVDAShNB4DgWFU6dnneABZWT3jucdJxnLAnnm0rORC3SNzMCxgghCw1ja7ioUMY9uKsBiJU2aElVFDEXL67CBqGkbJDG2AJqm5lZouacWHfVb1onKdp223blyo-b1x5Gz7rsMZ3XkeQbLA49NnmDwJXdF1qz9DKeowkldS+4bqiNM4wBHTpZvmxaVp8t8P1uPbi0OnkuIWJEoQ2DY4vsQU4OFOUlXkFw4osLtIneyn+p4b7ackenGaBlgQbBl4IY5z8Svh-8yoOoCNwcAyYjssJG0lFiLIRXobPLaIImDOFFiV0TPtVmmjQuEhGH4JkZrmhanmWiH8MIkjCLhyjTcR0KQSIAmhlFRsBkjetnexIYiFcdsBn9OEZbJzVeuVv3BoDyQg5DsOWR10HwZ82PiOuHbp25nSLZ5ax7sbSYsRcUZroDfOlW3CVIzM1UK5dH3+2w2BxsmiBk0kE1eEHc1hGIdf-s3o0+-KxAy4WAnVUjEY62hW7C-YjHOyWUYYhXrDMApDfKC35gRpUzoEKMUMolQai-xPv-M+JttIXwQKYAyZYGqhCGC4G6+dzrzFMJEHE1ZpaVyyjXH+EAGb1G3hgXeZoLTEDIYzMAsCk7wPNj0RwJ1oyqkSo4aITg4LYiLohX0KFYhf11PQihgCd5pjAZmbMtQJGECYeyM2644puxGJMJYJkHAcSMFiEM1ZMaRCHpiSUYiBx4GDgwUONIgHcD3gIQ+RArFNyUZIc+rDL6ynOpVAYSIIJlmPF2csY8RieGatCMYFjsKuJsUyKRVCZEZggTmFx1jbGMI8XA4KailQLHlJ2L0LYInBPlBWVU3ExjCkqTEn+1MfoYDpLAWAAB3IoEB3hwHqMzSO0cfIKSUmpDSvkpKfk8UjSqRB9K1X8ZKdwMRLJehRFGOsfirDMTqeSBp6sml4Bae09AnTuk4GBm3fWAzFIqXUnOSS0kJmhUxAZQIWjJhmBdHo3osQTomNMJjGIEE1hbKIOgE5djJDNLaR06h+9aEgpOUaSFhyIAPMOi6E6VYx7u2LljY8IQQwXiMshYu78bDAtBWgfUiT0BIuhck8BWZIEUvqIi-ZUKjmootm-HcUYqx1kjBsSyfQUSxDFtozO1hgXIFUBABJhpJDvDICOWAMKnGWmlbK9xGBFXKs5byPo8si4j0KcsMVx4JSDGvPpSCfQzJSplXKo0Oq4DANASkxlaSNX7CdUquAeqnRjFCNMuyr02JO04r44IJ5+jy09C6dUGpKBkDIfAH4xD0iHGoDhEcKiU6HRMNESwywuwDEqh6eKztmpBFxFBFY8tzpDC-pmrI9QaTNFzTzC22J8lz2hO-Wy2JAw8LlOiT0VazK+ibZkDt-dgSCnmL2kwEQB2YJmGGHcaJkqRj9GhTKvYSHkkpHgakBo6QMkoM3GdCC+aYyCHWasFgXD9BcCEQMSIQzywhMXZw+lVREP3b7H+SZqWpoRp23kxdQkQQhJ6dsEYp4zDWPdAmEFGwE2hLw4F7kr1eLohecEazoxREFGLWC+csYhickhFD3p3TAp2aNP+01zYsKRudcs2dGINrhGEXGqoi1OU9PLfcOd6P+0aegTWFDAKsdCuxhifEnJ+icCBVdRgwjzANTLM68tYpibrhJxu8TO2yd5uda2HpYp+gcjLcNRh5QnWLmLDYlUoSxDJXu6ugHD1-wAfKjAOH1wjDBOh8z0Z0r+mMLdJyCwNhOS7EMdR3ZPMU280QRRlD0CBdosF8E98ohl0bH0KY08YhF2lpU5YUFJjAribYzL2X82nigg+5dthmrFwSpLNYMtlTth6xefTatWUHI6V0yljWB7YKLi6Eu8ozIuhK5xHdKzljcRFA4JUxhyVgsy7So5k22EhnYt+3ORlFgYjxcXQynCRZmGOvazVmXnWgeTuBgNjE5Sobiu6R6dnejCjvS2GW0QLxvRSyJVemBDsBuWEEbigpR2Pv+4sUCMtNj8o2VjZL5NIcw6OhEMECPQ3I8DPMyw6PPQLKhPubbCQ4hAA */
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
          getUserCount: {
            data: TypesGen.UserCountResponse
          }
        },
      },
      predictableActionArguments: true,
      id: "usersState",
      type: "parallel",
      states: {
        count: {
          initial: "gettingCount",
          states: {
            idle: {},
            gettingCount: {
              entry: "clearGetCountError",
              invoke: {
                src: "getUserCount",
                id: "getUserCount",
                onDone: [
                  {
                    target: "idle",
                    actions: "assignCount",
                  },
                ],
                onError: [
                  {
                    target: "idle",
                    actions: "assignGetCountError",
                  },
                ],
              },
            },
          },
          on: {
            UPDATE_FILTER: {
              target: ".gettingCount",
              actions: ["assignFilter", "sendResetPage"],
            },
          },
        },
        users: {
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
                    actions: [
                      "assignDeleteUserError",
                      "displayDeleteErrorMessage",
                    ],
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
        getUserCount: (context) => {
          return API.getUserCount(queryToFilter(context.filter))
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
        assignCount: assign({
          count: (_, event) => event.data.count,
        }),
        assignGetCountError: assign({
          getCountError: (_, event) => event.data,
        }),
        clearGetCountError: assign({
          getCountError: (_) => undefined,
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
