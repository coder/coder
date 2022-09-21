import { getUsers } from "api/api"
import { User } from "api/typesGenerated"
import { queryToFilter } from "util/filters"
import { assign, createMachine } from "xstate"

export const searchUserMachine = createMachine(
  {
    id: "searchUserMachine",
    schema: {
      context: {} as {
        searchResults: User[]
      },
      events: {} as {
        type: "SEARCH"
        query: string
      },
      services: {} as {
        searchUsers: {
          data: User[]
        }
      },
    },
    context: {
      searchResults: [],
    },
    tsTypes: {} as import("./searchUserXService.typegen").Typegen0,
    initial: "idle",
    states: {
      idle: {
        on: {
          SEARCH: "searching",
        },
      },
      searching: {
        invoke: {
          src: "searchUsers",
          onDone: {
            target: "idle",
            actions: ["assignSearchResults"],
          },
        },
      },
    },
  },
  {
    services: {
      searchUsers: (_, { query }) => getUsers(queryToFilter(query)),
    },
    actions: {
      assignSearchResults: assign({
        searchResults: (_, { data }) => data,
      }),
    },
  },
)
