import { assign, createMachine } from "xstate"
import * as API from "../../api"
import { ApiError, FieldErrors, isApiError, mapApiErrorToFieldErrors } from "../../api/errors"
import * as Types from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"
import { displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  createUserSuccess: "Successfully created user.",
}

export interface UsersContext {
  users?: TypesGen.User[]
  getUsersError?: Error | unknown
  createUserError?: Error | unknown
  createUserFormErrors?: FieldErrors
}

export type UsersEvent = { type: "GET_USERS" } | { type: "CREATE"; user: Types.CreateUserRequest }

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
      },
    },
    id: "usersState",
    context: {
      users: [],
    },
    initial: "idle",
    states: {
      idle: {
        on: {
          GET_USERS: "gettingUsers",
          CREATE: "creatingUser",
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
      error: {
        on: {
          GET_USERS: "gettingUsers",
        },
      },
    },
  },
  {
    services: {
      getUsers: API.getUsers,
      createUser: (_, event) => API.createUser(event.user),
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
      clearCreateUserError: assign((context: UsersContext) => ({
        ...context,
        createUserError: undefined,
      })),
      displayCreateUserSuccess: () => {
        displaySuccess(Language.createUserSuccess)
      },
    },
  },
)
