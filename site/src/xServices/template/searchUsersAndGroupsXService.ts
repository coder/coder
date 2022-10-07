import { getGroups, getUsers } from "api/api"
import { Group, User } from "api/typesGenerated"
import { queryToFilter } from "util/filters"
import { assign, createMachine } from "xstate"

export type SearchUsersAndGroupsEvent =
  | { type: "SEARCH"; query: string }
  | { type: "CLEAR_RESULTS" }

export const searchUsersAndGroupsMachine = createMachine(
  {
    id: "searchUsersAndGroups",
    predictableActionArguments: true,
    schema: {
      context: {} as {
        organizationId: string
        userResults: User[]
        groupResults: Group[]
      },
      events: {} as SearchUsersAndGroupsEvent,
      services: {} as {
        search: {
          data: {
            users: User[]
            groups: Group[]
          }
        }
      },
    },
    tsTypes: {} as import("./searchUsersAndGroupsXService.typegen").Typegen0,
    initial: "idle",
    states: {
      idle: {
        on: {
          SEARCH: {
            target: "searching",
            cond: "queryHasMinLength",
          },
          CLEAR_RESULTS: {
            actions: ["clearResults"],
            target: "idle",
          },
        },
      },
      searching: {
        invoke: {
          src: "search",
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
      search: async ({ organizationId }, { query }) => {
        const [users, groups] = await Promise.all([
          getUsers(queryToFilter(query)),
          getGroups(organizationId),
        ])

        return { users, groups }
      },
    },
    actions: {
      assignSearchResults: assign({
        userResults: (_, { data }) => data.users,
        groupResults: (_, { data }) => data.groups,
      }),
      clearResults: assign({
        userResults: (_) => [],
        groupResults: (_) => [],
      }),
    },
    guards: {
      queryHasMinLength: (_, { query }) => query.length >= 3,
    },
  },
)
