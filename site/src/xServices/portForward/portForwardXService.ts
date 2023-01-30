import { getAgentListeningPorts } from "api/api"
import { WorkspaceAgentListeningPortsResponse } from "api/typesGenerated"
import { createMachine, assign } from "xstate"

export const portForwardMachine = createMachine(
  {
    predictableActionArguments: true,
    id: "portForwardMachine",
    schema: {
      context: {} as {
        agentId: string
        listeningPorts?: WorkspaceAgentListeningPortsResponse
      },
      services: {} as {
        getListeningPorts: {
          data: WorkspaceAgentListeningPortsResponse
        }
      },
    },
    tsTypes: {} as import("./portForwardXService.typegen").Typegen0,
    initial: "loading",
    states: {
      loading: {
        invoke: {
          src: "getListeningPorts",
          onDone: {
            target: "success",
            actions: ["assignListeningPorts"],
          },
        },
      },
      success: {
        type: "final",
      },
    },
  },
  {
    services: {
      getListeningPorts: ({ agentId }) => getAgentListeningPorts(agentId),
    },
    actions: {
      assignListeningPorts: assign({
        listeningPorts: (_, { data }) => data,
      }),
    },
  },
)
