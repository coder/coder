import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { ApiError, FieldErrors, isApiError, mapApiErrorToFieldErrors } from "../../api/errors"
import * as TypesGen from "../../api/typesGenerated"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"
import { generateRandomString } from "../../util/random"

export const Language = {
  createUserSuccess: "Successfully created user.",
  suspendUserSuccess: "Successfully suspended the user.",
  suspendUserError: "Error on suspend the user.",
  resetUserPasswordSuccess: "Successfully updated the user password.",
  resetUserPasswordError: "Error on reset the user password.",
  updateUserRolesSuccess: "Successfully updated the user roles.",
  updateUserRolesError: "Error on update the user roles.",
}

export interface UsersContext {
  // Get users
  users?: TypesGen.User[]
  getUsersError?: Error | unknown
  createUserError?: Error | unknown
  createUserFormErrors?: FieldErrors
  // Suspend user
  userIdToSuspend?: TypesGen.User["id"]
  suspendUserError?: Error | unknown
  // Reset user password
  userIdToResetPassword?: TypesGen.User["id"]
  resetUserPasswordError?: Error | unknown
  newUserPassword?: string
  // Update user roles
  userIdToUpdateRoles?: TypesGen.User["id"]
  updateUserRolesError?: Error | unknown
}

export type UsersEvent =
  | { type: "GET_USERS" }
  | { type: "CREATE"; user: TypesGen.CreateUserRequest }
  // Suspend events
  | { type: "SUSPEND_USER"; userId: TypesGen.User["id"] }
  | { type: "CONFIRM_USER_SUSPENSION" }
  | { type: "CANCEL_USER_SUSPENSION" }
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
          GET_USERS: "gettingUsers",
          CREATE: "creatingUser",
          SUSPEND_USER: {
            target: "confirmUserSuspension",
            actions: ["assignUserIdToSuspend"],
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
              actions: "assignGetUsersError",
              target: "#usersState.error",
            },
          ],
        },
        tags: "loading",
      },
      creatingUser: {
        invoke: {
          src: "createUser",
          id: "createUser",
          onDone: {
            target: "idle",
            actions: ["displayCreateUserSuccess", "redirectToUsersPage", "clearCreateUserError"],
          },
          onError: [
            {
              target: "idle",
              cond: "isFormError",
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
          GET_USERS: "gettingUsers",
        },
      },
    },
  },
  {
    services: {
      // Passing API.getUsers directly does not invoke the function properly
      // when it is mocked. This happen in the UsersPage tests inside of the
      // "shows a success message and refresh the page" test case.
      getUsers: () => API.getUsers(),
      createUser: (_, event) => API.createUser(event.user),
      suspendUser: (context) => {
        if (!context.userIdToSuspend) {
          throw new Error("userIdToSuspend is undefined")
        }

        return API.suspendUser(context.userIdToSuspend)
      },
      resetUserPassword: (context) => {
        if (!context.userIdToResetPassword) {
          throw new Error("userIdToResetPassword is undefined")
        }

        if (!context.newUserPassword) {
          throw new Error("newUserPassword not generated")
        }

        return API.updateUserPassword(context.newUserPassword, context.userIdToResetPassword)
      },
      updateUserRoles: (context, event) => {
        if (!context.userIdToUpdateRoles) {
          throw new Error("userIdToUpdateRoles is undefined")
        }

        return API.updateUserRoles(event.roles, context.userIdToUpdateRoles)
      },
    },
    guards: {
      isFormError: (_, event) => isApiError(event.data),
    },
    actions: {
      assignUsers: assign({
        users: (_, event) => event.data,
      }),
      assignGetUsersError: assign({
        getUsersError: (_, event) => event.data,
      }),
      assignUserIdToSuspend: assign({
        userIdToSuspend: (_, event) => event.userId,
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
        createUserError: (_, event) => event.data,
      }),
      assignCreateUserFormErrors: assign({
        // the guard ensures it is ApiError
        createUserFormErrors: (_, event) => mapApiErrorToFieldErrors((event.data as ApiError).response.data),
      }),
      assignSuspendUserError: assign({
        suspendUserError: (_, event) => event.data,
      }),
      assignResetUserPasswordError: assign({
        resetUserPasswordError: (_, event) => event.data,
      }),
      assignUpdateRolesError: assign({
        updateUserRolesError: (_, event) => event.data,
      }),
      clearCreateUserError: assign((context: UsersContext) => ({
        ...context,
        createUserError: undefined,
      })),
      clearSuspendUserError: assign({
        suspendUserError: (_) => undefined,
      }),
      clearResetUserPasswordError: assign({
        resetUserPasswordError: (_) => undefined,
      }),
      clearUpdateUserRolesError: assign({
        updateUserRolesError: (_) => undefined,
      }),
      displayCreateUserSuccess: () => {
        displaySuccess(Language.createUserSuccess)
      },
      displaySuspendSuccess: () => {
        displaySuccess(Language.suspendUserSuccess)
      },
      displaySuspendedErrorMessage: () => {
        displayError(Language.suspendUserError)
      },
      displayResetPasswordSuccess: () => {
        displaySuccess(Language.resetUserPasswordSuccess)
      },
      displayResetPasswordErrorMessage: () => {
        displayError(Language.resetUserPasswordError)
      },
      displayUpdateRolesErrorMessage: () => {
        displayError(Language.updateUserRolesError)
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
