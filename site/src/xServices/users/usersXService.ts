import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { getErrorMessage } from "../../api/errors"
import * as TypesGen from "../../api/typesGenerated"
import {
  displayError,
  displaySuccess,
} from "../../components/GlobalSnackbar/utils"
import { queryToFilter } from "../../util/filters"
import { generateRandomString } from "../../util/random"

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
  filter?: string
  getUsersError?: Error | unknown
  // Suspend user
  userIdToSuspend?: TypesGen.User["id"]
  suspendUserError?: Error | unknown
  // Delete user
  userIdToDelete?: TypesGen.User["id"]
  deleteUserError?: Error | unknown
  // Activate user
  userIdToActivate?: TypesGen.User["id"]
  activateUserError?: Error | unknown
  // Reset user password
  userIdToResetPassword?: TypesGen.User["id"]
  resetUserPasswordError?: Error | unknown
  newUserPassword?: string
  // Update user roles
  userIdToUpdateRoles?: TypesGen.User["id"]
  updateUserRolesError?: Error | unknown
}

export type UsersEvent =
  | { type: "GET_USERS"; query?: string }
  // Suspend events
  | { type: "SUSPEND_USER"; userId: TypesGen.User["id"] }
  | { type: "CONFIRM_USER_SUSPENSION" }
  | { type: "CANCEL_USER_SUSPENSION" }
  // Delete events
  | { type: "DELETE_USER"; userId: TypesGen.User["id"] }
  | { type: "CONFIRM_USER_DELETE" }
  | { type: "CANCEL_USER_DELETE" }
  // Activate events
  | { type: "ACTIVATE_USER"; userId: TypesGen.User["id"] }
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

export const usersMachine = createMachine(
  {
    id: "usersState",
    predictableActionArguments: true,
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
    initial: "gettingUsers",
    states: {
      gettingUsers: {
        entry: "clearGetUsersError",
        invoke: {
          src: "getUsers",
          id: "getUsers",
          onDone: [
            {
              target: "#usersState.idle",
              actions: "assignUsers",
            },
          ],
          onError: [
            {
              actions: [
                "clearUsers",
                "assignGetUsersError",
                "displayGetUsersErrorMessage",
              ],
              target: "#usersState.error",
            },
          ],
        },
        tags: "loading",
      },
      idle: {
        on: {
          GET_USERS: {
            actions: "assignFilter",
            target: "gettingUsers",
          },
          SUSPEND_USER: {
            target: "confirmUserSuspension",
            actions: ["assignUserIdToSuspend"],
          },
          DELETE_USER: {
            target: "confirmUserDeletion",
            actions: ["assignUserIdToDelete"],
          },
          ACTIVATE_USER: {
            target: "confirmUserActivation",
            actions: ["assignUserIdToActivate"],
          },
          RESET_USER_PASSWORD: {
            target: "confirmUserPasswordReset",
            actions: ["assignUserIdToResetPassword", "generateRandomPassword"],
          },
          UPDATE_USER_ROLES: {
            target: "updatingUserRoles",
            actions: ["assignUserIdToUpdateRoles"],
          },
        },
      },
      confirmUserSuspension: {
        on: {
          CONFIRM_USER_SUSPENSION: "suspendingUser",
          CANCEL_USER_SUSPENSION: "idle",
        },
      },
      confirmUserDeletion: {
        on: {
          CONFIRM_USER_DELETE: "deletingUser",
          CANCEL_USER_DELETE: "idle",
        },
      },
      confirmUserActivation: {
        on: {
          CONFIRM_USER_ACTIVATION: "activatingUser",
          CANCEL_USER_ACTIVATION: "idle",
        },
      },
      suspendingUser: {
        entry: "clearSuspendUserError",
        invoke: {
          src: "suspendUser",
          id: "suspendUser",
          onDone: {
            // Update users list
            target: "gettingUsers",
            actions: ["displaySuspendSuccess"],
          },
          onError: {
            target: "idle",
            actions: ["assignSuspendUserError", "displaySuspendedErrorMessage"],
          },
        },
      },
      deletingUser: {
        entry: "clearDeleteUserError",
        invoke: {
          src: "deleteUser",
          id: "deleteUser",
          onDone: {
            target: "gettingUsers",
            actions: ["displayDeleteSuccess"],
          },
          onError: {
            target: "idle",
            actions: ["assignDeleteUserError", "displayDeleteErrorMessage"],
          },
        },
      },
      activatingUser: {
        entry: "clearActivateUserError",
        invoke: {
          src: "activateUser",
          id: "activateUser",
          onDone: {
            // Update users list
            target: "gettingUsers",
            actions: ["displayActivateSuccess"],
          },
          onError: {
            target: "idle",
            actions: [
              "assignActivateUserError",
              "displayActivatedErrorMessage",
            ],
          },
        },
      },
      confirmUserPasswordReset: {
        on: {
          CONFIRM_USER_PASSWORD_RESET: "resettingUserPassword",
          CANCEL_USER_PASSWORD_RESET: "idle",
        },
      },
      resettingUserPassword: {
        entry: "clearResetUserPasswordError",
        invoke: {
          src: "resetUserPassword",
          id: "resetUserPassword",
          onDone: {
            target: "idle",
            actions: ["displayResetPasswordSuccess"],
          },
          onError: {
            target: "idle",
            actions: [
              "assignResetUserPasswordError",
              "displayResetPasswordErrorMessage",
            ],
          },
        },
      },
      updatingUserRoles: {
        entry: "clearUpdateUserRolesError",
        invoke: {
          src: "updateUserRoles",
          id: "updateUserRoles",
          onDone: {
            target: "idle",
            actions: ["updateUserRolesInTheList"],
          },
          onError: {
            target: "idle",
            actions: [
              "assignUpdateRolesError",
              "displayUpdateRolesErrorMessage",
            ],
          },
        },
      },
      error: {
        on: {
          GET_USERS: {
            actions: "assignFilter",
            target: "gettingUsers",
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
      getUsers: (context) => API.getUsers(queryToFilter(context.filter)),
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
      assignUsers: assign({
        users: (_, event) => event.data,
      }),
      assignFilter: assign({
        filter: (_, event) => event.query,
      }),
      assignGetUsersError: assign({
        getUsersError: (_, event) => event.data,
      }),
      assignUserIdToSuspend: assign({
        userIdToSuspend: (_, event) => event.userId,
      }),
      assignUserIdToDelete: assign({
        userIdToDelete: (_, event) => event.userId,
      }),
      assignUserIdToActivate: assign({
        userIdToActivate: (_, event) => event.userId,
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
    },
  },
)
