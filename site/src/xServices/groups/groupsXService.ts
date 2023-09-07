import { getGroups } from "api/api";
import { getErrorMessage } from "api/errors";
import { Group } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { assign, createMachine } from "xstate";

export const groupsMachine = createMachine(
  {
    id: "groupsMachine",
    predictableActionArguments: true,
    schema: {
      context: {} as {
        organizationId: string;
        shouldFetchGroups: boolean;
        groups?: Group[];
      },
      services: {} as {
        loadGroups: {
          data: Group[];
        };
      },
    },
    tsTypes: {} as import("./groupsXService.typegen").Typegen0,
    initial: "loading",
    states: {
      loading: {
        always: [{ target: "idle", cond: "cantFetchGroups" }],
        invoke: {
          src: "loadGroups",
          onDone: {
            actions: ["assignGroups"],
            target: "idle",
          },
          onError: {
            target: "idle",
            actions: ["displayLoadingGroupsError"],
          },
        },
      },
      idle: {},
    },
  },
  {
    guards: {
      cantFetchGroups: ({ shouldFetchGroups }) => !shouldFetchGroups,
    },
    services: {
      loadGroups: ({ organizationId }) => getGroups(organizationId),
    },
    actions: {
      assignGroups: assign({
        groups: (_, { data }) => data,
      }),
      displayLoadingGroupsError: (_, { data }) => {
        const message = getErrorMessage(data, "Error on loading groups.");
        displayError(message);
      },
    },
  },
);
