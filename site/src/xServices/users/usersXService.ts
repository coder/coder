import { assign, createMachine } from "xstate"
import * as API from "../../api"
import { ApiError, FieldErrors, isApiError, mapApiErrorToFieldErrors } from "../../api/errors"
import * as Types from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  createUserSuccess: "Successfully created user.",
  suspendUserSuccess: "Successfully suspended the user.",
  suspendUserError: "Error on suspend the user",
}

export interface UsersContext {
  users?: TypesGen.User[]
  userIdToSuspend?: TypesGen.User["id"]
  getUsersError?: Error | unknown
  createUserError?: Error | unknown
  createUserFormErrors?: FieldErrors
  suspendUserError?: Error | unknown
}

export type UsersEvent =
  | { type: "GET_USERS" }
  | { type: "CREATE"; user: Types.CreateUserRequest }
  | { type: "SUSPEND_USER"; userId: TypesGen.User["id"] }
  | { type: "CONFIRM_USER_SUSPENSION" }
  | { type: "CANCEL_USER_SUSPENSION" }

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
      },
    },
    id: "usersState",
    initial: "idle",
    context: {
      users: [],
    },
    states: {
      idle: {
        on: {
          GET_USERS: "gettingUsers",
          CREATE: "creatingUser",
          SUSPEND_USER: {
            target: "confirmUserSuspension",
            actions: ["assignUserIdToSuspend"],
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
      clearCreateUserError: assign((context: UsersContext) => ({
        ...context,
        createUserError: undefined,
      })),
      clearSuspendUserError: assign({
        suspendUserError: (_) => undefined,
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
    },
  },
)
