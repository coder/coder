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
  /** @xstate-layout N4IgpgJg5mDOIC5QFdZgE6wMoBcCGOYAdLPujgJYB2UACnlNQRQPZUDEA2gAwC6ioAA4tYFSmwEgAnogCM3AJzciAZm4AmAKwAOAOwA2TQt0AWfQBoQAD0QBaWbIWyimzepPqtCpd1n6Avv6WqBjY+IREMDiUNACqaJjsEGzE1ABuLADWxFHxoTz8SCDCouJUkjIIsuoq+kRmmrreJtqertqWNgj2stoKRLq13upOspq+AUEgIZi4BDlg0dRQeYkY6CzoRIIANgQAZpsAtpGLq7AFkiVirOVFXT19A0MKIw7jfpaVDtra9YZNBQtNo6QLBBJheZECgQHZgdhYWJYWgAUQAcgARAD6SJRACVLkVrmVJA8HMYBkCFCptH4Rvp9CYvnI1P1qbJTL1DNx9NoVGDphC5hEYXD2BiUQAZFEAFRROKw+MJQhENwk9zs5N0lJM1Np+npjOZVRpKlUxl5HhMul+3G0ApmkJFsPhAEEAMIygCSADVXXKFUq+FdVSSNd0tTq9XSFAymdI5CZNHVtPpauoGWp9NT+VNHcLUi72HiUYqZYG8VjaK6sFgAOoAeTxGOVxVDt1Jmsc2qauppMbjxrGalUI2TSlkKnGoLzQvChbFsVoGP98txlbxDelWFbxI74Z6FN70YNsaNCaq+kcRDTThUfjPDnUDrnUNF8KXK4D1YA4ijd+26qgGS3ZRv2p6DheD5mvI4ymNwvY1LoL6hAW0JFp+q5YgAYl6kpygSwZEoBdzAV2R5UuBhrxt8Bh1I08h+GYnjpihszzkQADGbD7BQ6BHKsWCoIIYBUKIbDsO6DZorheIALIVliiLIuiWBetJAGlPuZERuo3AmEQfiaCoCipoMnj6UO2iuEQtqNE4SYqCYHJsU6xDcVQvH8YJwmieJHDuq6aLulKinKaiaJqRpREqlpQHWJqekGUZJlmSoFk0XISjqDeuieK81nNJOrloR5XkCQkGJgHCZSSdJskKeuWIStKcqaWqpEJbpHJEIoV5+LIznAkOVL-CY3A8o0SbOSVHFlXxFUYFVNW3JJQUhZKiktbK-4xW2cWdWS6g9X1DhXkNrQjbGRBaI0enqH0g3cJos1QvN3kJK6nGUGkzASVJMlevJiket6fretFhSxR1nbdcoagtBoxgTQomiZVUo1mONk2mGjsivRE72LegX0-X9AXraFTWg76-rqWi7Vhjp9jHfD+naEjugo2jV11LdeUaI940vbOqEcbAvlUBAyyrEkKTQlQGTZCQksQKsjPaVBPguKmg3UgyD0KJ0Xb6MovhDPZNRXgTxAS7AIlSzLCTsOsmzbHsOCHPxKv26JasJBr8UgabvWTtmlu1LIVluDr536wafQ20QEDVYsTsYHLVCpIrWTECnNVgOre17kHXa6JoN2xn4jKoy0Fha6YvVuIoHK6oxIvgmLUL52ncTO67Wy7AcxzJ6nhBF1D+0wweDjl5XV5xrXqbGkl2qaL03BOZOjiDUneDfRQv0xCszvJFnCtK8Q+9k+PAfFyRsM9IyN6DQ4NJ8r0k4jY3z16U4phOF8B3QUXcIjX0PswPuGcB7u2Ht7cBR9C530niXQ6JsDLnTfr8e8tIVAr30mvDeW97wOSTkTVY9BYCwAAO6bAgHiOAiw6qA2Bk1astZGzNixCWMsgc0ERl6P0deKNrSfw8OoFeYdbLqHLrGWkHwFBkJ4gtCheAqG0PQPQxhOA1rBSpoqSs7D6xNmxDw2UfDH4-CEfIRQojaTiMkXyF+7Jf5V0UaLdiUJ0DaOPqo9RdDM7Z0vkQbxaAcB+JoXQixRRKhaH0k3QYNonCowcPGECvQbz6AMA4bsaYTAmCTqExYviEiUMiZol26ANiDw9l7E4RTwmlLUeUiA0TmY5L+AybJWo8no3cMmBJKgkmOHXrvDxbkiDIEEBASBJ8MB4hYHCWAgSL650mdM+YqwFlLLaV1Ho94BhNF+MZLJrg0wr0aGvGo6VJwaHSgU8ZaEpkzJKfMxZcBKnVNgZ7EezzNkJG2XAXZIEDm6COdZWo5dkx4IvBmcYRBTKvDTLydetQHmd08YQdgmEAy4XwkGFBD8Yl2F0M4IYTkHqb1RuMDoXUzBmyaPpPJpyVCssCFMKgLAU7wCKPmcWZBj70EYFQcmIYDqWPyc4XUCErx8ncEORQbMtB6EMMYMwScoivMwGK6e7TJX1CUNkuV6Nqi1H+PZIErRbr2keRxd8OqmZ7MGs5A1Mrej3KHKlXq+UHrv2nDajFEzyEJCEr7MSmtUESpddKo1HqoJo2Srrcax127AL5W9ZRH0lpjwjUSvV0bDWyrjd8A2CLdbPXeBzBCSjPIqM+gfI+ubxUz31TGot8qoI2mcDaK8Mj3DyHyQGkBmLbaq3TugB1msyStsLe6jt3xtY6DjrUBO7jA1oR7lqydpcIwzrdca-BBpDImBuXlIZNItB7wbbM1Y27+H2D3bG+diBPA1AGBNB6wjtDWgNDW8qESNFaLCXeqNUrZ0HthTSCuWhOTjUGnlX9tqvE+PHWUwDIGW0Fv3cWl94wDLPUSY9FJYz10cT+VqwFPLoaOunVhp9fSGQV3OnBVojRzo2ww3qp4ba53o3sCHLeRhUWm0GKmdl-ggA */
  createMachine(
    {
      tsTypes: {} as import("./usersXService.typegen").Typegen0,
      schema: {
        context: {} as UsersContext,
        events: {} as UsersEvent,
        services: {} as {
          getUsers: {
            data: TypesGen.GetUsersResponse
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
      on: {
        UPDATE_FILTER: {
          actions: ["assignFilter", "sendResetPage"],
          internal: false,
        },
        UPDATE_PAGE: {
          target: "gettingUsers",
          actions: "updateURL",
        },
      },
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
          users: (_, event) => event.data.users,
          count: (_, event) => event.data.count,
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
