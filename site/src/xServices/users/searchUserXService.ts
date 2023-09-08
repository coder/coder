import { getUsers } from "api/api";
import { User } from "api/typesGenerated";
import { queryToFilter } from "utils/filters";
import { assign, createMachine } from "xstate";

export type AutocompleteEvent =
  | { type: "SEARCH"; query: string }
  | { type: "CLEAR_RESULTS" };

export const searchUserMachine = createMachine(
  {
    id: "searchUserMachine",
    predictableActionArguments: true,
    schema: {
      context: {} as {
        searchResults?: User[];
      },
      events: {} as AutocompleteEvent,
      services: {} as {
        searchUsers: {
          data: User[];
        };
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
          CLEAR_RESULTS: {
            actions: ["clearResults"],
            target: "idle",
          },
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
      searchUsers: async (_, { query }) =>
        (await getUsers(queryToFilter(query))).users,
    },
    actions: {
      assignSearchResults: assign({
        searchResults: (_, { data }) => data,
      }),
      clearResults: assign({
        searchResults: (_) => undefined,
      }),
    },
  },
);
