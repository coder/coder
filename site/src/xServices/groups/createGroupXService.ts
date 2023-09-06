import { createGroup } from "api/api";
import { CreateGroupRequest, Group } from "api/typesGenerated";
import { createMachine, assign } from "xstate";

export const createGroupMachine = createMachine(
  {
    id: "createGroupMachine",
    schema: {
      context: {} as {
        organizationId: string;
        error?: unknown;
      },
      services: {} as {
        createGroup: {
          data: Group;
        };
      },
      events: {} as {
        type: "CREATE";
        data: CreateGroupRequest;
      },
    },
    tsTypes: {} as import("./createGroupXService.typegen").Typegen0,
    initial: "idle",
    states: {
      idle: {
        on: {
          CREATE: {
            target: "creatingGroup",
          },
        },
      },
      creatingGroup: {
        invoke: {
          src: "createGroup",
          onDone: {
            target: "idle",
            actions: ["onCreate"],
          },
          onError: {
            target: "idle",
            actions: ["assignError"],
          },
        },
      },
    },
  },
  {
    services: {
      createGroup: ({ organizationId }, { data }) =>
        createGroup(organizationId, data),
    },
    actions: {
      assignError: assign({
        error: (_, event) => event.data,
      }),
    },
  },
);
