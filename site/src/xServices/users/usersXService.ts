import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"

export interface UsersContext {
  users: Types.UserResponse[]
  pager?: Types.Pager
  getUsersError?: Error | unknown
}

export type UsersEvent = { type: "GET_USERS" }

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
      },
    },
    id: "usersState",
    context: {
      users: [],
    },
    initial: "gettingUsers",
    states: {
      gettingUsers: {
        invoke: {
          src: "getUsers",
          id: "getUsers",
          onDone: [
            {
              target: "#usersState.ready",
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
      ready: {
        on: {
          GET_USERS: "gettingUsers",
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
    },
  },
)
