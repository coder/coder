import { getGroups, getTemplateACLAvailable, getUsers } from "api/api";
import { Group, User } from "api/typesGenerated";
import { queryToFilter } from "utils/filters";
import { everyOneGroup } from "utils/groups";
import { assign, createMachine } from "xstate";

export type SearchUsersAndGroupsEvent =
  | { type: "SEARCH"; query: string }
  | { type: "CLEAR_RESULTS" };

export const searchUsersAndGroupsMachine = createMachine(
  {
    id: "searchUsersAndGroups",
    predictableActionArguments: true,
    schema: {
      context: {} as {
        organizationId: string;
        templateID?: string;
        userResults: User[];
        groupResults: Group[];
      },
      events: {} as SearchUsersAndGroupsEvent,
      services: {} as {
        search: {
          data: {
            users: User[];
            groups: Group[];
          };
        };
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
      search: async ({ organizationId, templateID }, { query }) => {
        let users, groups;
        if (templateID && templateID !== "") {
          const res = await getTemplateACLAvailable(
            templateID,
            queryToFilter(query),
          );
          users = res.users;
          groups = res.groups;
        } else {
          const [userRes, groupsRes] = await Promise.all([
            getUsers(queryToFilter(query)),
            getGroups(organizationId),
          ]);

          users = userRes.users;
          groups = groupsRes;
        }

        // The Everyone groups is not returned by the API so we have to add it
        // manually
        return {
          users: users,
          groups: [everyOneGroup(organizationId), ...groups],
        };
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
);
