import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import {
  ApiError,
  FieldErrors,
  getErrorMessage,
  hasApiFieldErrors,
  isApiError,
  mapApiErrorToFieldErrors,
} from "../../api/errors"
import * as TypesGen from "../../api/typesGenerated"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"
import { queryToFilter } from "../../util/filters"
import { generateRandomString } from "../../util/random"

export const Language = {
  getUsersError: "Error getting users.",
  createUserSuccess: "Successfully created user.",
  createUserError: "Error on creating the user.",
  suspendUserSuccess: "Successfully suspended the user.",
  suspendUserError: "Error suspending user.",
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
  createUserErrorMessage?: string
  createUserFormErrors?: FieldErrors
  // Suspend user
  userIdToSuspend?: TypesGen.User["id"]
  suspendUserError?: Error | unknown
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
  | { type: "GET_USERS"; query: string }
  | { type: "CREATE"; user: TypesGen.CreateUserRequest }
  | { type: "CANCEL_CREATE_USER" }
  // Suspend events
  | { type: "SUSPEND_USER"; userId: TypesGen.User["id"] }
  | { type: "CONFIRM_USER_SUSPENSION" }
  | { type: "CANCEL_USER_SUSPENSION" }
  // Activate events
  | { type: "ACTIVATE_USER"; userId: TypesGen.User["id"] }
  | { type: "CONFIRM_USER_ACTIVATION" }
  | { type: "CANCEL_USER_ACTIVATION" }
  // Reset password events
  | { type: "RESET_USER_PASSWORD"; userId: TypesGen.User["id"] }
  | { type: "CONFIRM_USER_PASSWORD_RESET" }
  | { type: "CANCEL_USER_PASSWORD_RESET" }
  // Update roles events
  | { type: "UPDATE_USER_ROLES"; userId: TypesGen.User["id"]; roles: TypesGen.Role["name"][] }

export const usersMachine = createMachine(
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
    id: "usersState",
    initial: "idle",
    states: {
      idle: {
        on: {
          GET_USERS: {
            actions: "assignFilter",
            target: "gettingUsers",
          },
          CREATE: "creatingUser",
          CANCEL_CREATE_USER: { actions: ["clearCreateUserError"] },
          SUSPEND_USER: {
            target: "confirmUserSuspension",
            actions: ["assignUserIdToSuspend"],
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
      gettingUsers: {
        invoke: {
          src: "getUsers",
          id: "getUsers",
          onDone: [
            {
              target: "#usersState.idle",
              actions: ["assignUsers", "clearGetUsersError"],
            },
          ],
          onError: [
            {
              actions: ["assignGetUsersError", "displayGetUsersErrorMessage"],
              target: "#usersState.error",
            },
          ],
        },
        tags: "loading",
      },
      creatingUser: {
        entry: "clearCreateUserError",
        invoke: {
          src: "createUser",
          id: "createUser",
          onDone: {
            target: "idle",
            actions: ["displayCreateUserSuccess", "redirectToUsersPage"],
          },
          onError: [
            {
              target: "idle",
              cond: "hasFieldErrors",
              actions: ["assignCreateUserFormErrors"],
            },
            {
              target: "idle",
              actions: ["assignCreateUserError"],
            },
          ],
        },
        tags: "loading",
      },
      confirmUserSuspension: {
        on: {
          CONFIRM_USER_SUSPENSION: "suspendingUser",
          CANCEL_USER_SUSPENSION: "idle",
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
            actions: ["assignActivateUserError", "displayActivatedErrorMessage"],
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
            actions: ["assignResetUserPasswordError", "displayResetPasswordErrorMessage"],
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
            actions: ["assignUpdateRolesError", "displayUpdateRolesErrorMessage"],
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
      createUser: (_, event) => API.createUser(event.user),
      suspendUser: (context) => {
        if (!context.userIdToSuspend) {
          throw new Error("userIdToSuspend is undefined")
        }

        return API.suspendUser(context.userIdToSuspend)
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
    guards: {
      hasFieldErrors: (_, event) => isApiError(event.data) && hasApiFieldErrors(event.data),
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
      assignCreateUserError: assign({
        createUserErrorMessage: (_, event) => getErrorMessage(event.data, Language.createUserError),
      }),
      assignCreateUserFormErrors: assign({
        // the guard ensures it is ApiError
        createUserFormErrors: (_, event) =>
          mapApiErrorToFieldErrors((event.data as ApiError).response.data),
      }),
      assignSuspendUserError: assign({
        suspendUserError: (_, event) => event.data,
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
      clearCreateUserError: assign((context: UsersContext) => ({
        ...context,
        createUserErrorMessage: undefined,
        createUserFormErrors: undefined,
      })),
      clearSuspendUserError: assign({
        suspendUserError: (_) => undefined,
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
        const message = getErrorMessage(context.getUsersError, Language.getUsersError)
        displayError(message)
      },
      displayCreateUserSuccess: () => {
        displaySuccess(Language.createUserSuccess)
      },
      displaySuspendSuccess: () => {
        displaySuccess(Language.suspendUserSuccess)
      },
      displaySuspendedErrorMessage: (context) => {
        const message = getErrorMessage(context.suspendUserError, Language.suspendUserError)
        displayError(message)
      },
      displayActivateSuccess: () => {
        displaySuccess(Language.activateUserSuccess)
      },
      displayActivatedErrorMessage: (context) => {
        const message = getErrorMessage(context.activateUserError, Language.activateUserError)
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
        const message = getErrorMessage(context.updateUserRolesError, Language.updateUserRolesError)
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
