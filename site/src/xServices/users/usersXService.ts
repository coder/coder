import { assign, createMachine } from "xstate"
import * as API from "../../api"
import { ApiError, FieldErrors, isApiError, mapApiErrorToFieldErrors } from "../../api/errors"
import * as Types from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"
import { displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  createUserSuccess: "Successfully created user",
}

export interface UsersContext {
  users: Types.UserResponse[]
  pager?: Types.Pager
  getUsersError?: Error | unknown
  createUserError?: Error | unknown
  createUserFormErrors?: FieldErrors
}

export type UsersEvent =
  | { type: "GET_USERS" }
  | { type: "ENTER_CREATION_MODE" }
  | { type: "EXIT_CREATION_MODE" }
  | { type: "CREATE"; user: TypesGen.CreateUserRequest }

export const usersMachine = createMachine(
  {
    tsTypes: {} as import("./usersXService.typegen").Typegen0,
    schema: {
      context: {} as UsersContext,
      events: {} as UsersEvent,
      services: {} as {
        getUsers: {
          data: Types.PagedUsers
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
          ENTER_CREATION_MODE: "creationMode",
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
      creationMode: {
        initial: "idle",
        states: {
          idle: {
            on: {
              CREATE: "creatingUser",
              EXIT_CREATION_MODE: "#usersState.idle"
            },
          },
          creatingUser: {
            invoke: {
              src: "createUser",
              id: "createUser",
              onDone: {
                target: "#usersState.idle",
                actions: ["displayCreateUserSuccess", "clearCreateUserError"],
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
      getUsers: API.getUsers,
      createUser: (_, event) => API.createUser(event.user),
    },
    guards: {
      isFormError: (_, event) => isApiError(event.data),
    },
    actions: {
      assignUsers: assign({
        users: (_, event) => event.data.page,
        pager: (_, event) => event.data.pager,
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
