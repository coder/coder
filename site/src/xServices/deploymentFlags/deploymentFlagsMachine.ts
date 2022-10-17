import { getDeploymentFlags } from "api/api"
import { DeploymentFlags } from "api/typesGenerated"
import { createMachine, assign } from "xstate"

export const deploymentFlagsMachine = createMachine(
  {
    id: "deploymentFlagsMachine",
    initial: "idle",
    schema: {
      context: {} as {
        deploymentFlags?: DeploymentFlags
        getDeploymentFlagsError?: unknown
      },
      events: {} as { type: "LOAD" },
      services: {} as {
        getDeploymentFlags: {
          data: DeploymentFlags
        }
      },
    },
    tsTypes: {} as import("./deploymentFlagsMachine.typegen").Typegen0,
    states: {
      idle: {
        on: {
          LOAD: {
            target: "loading",
          },
        },
      },
      loading: {
        invoke: {
          src: "getDeploymentFlags",
          onDone: {
            target: "loaded",
            actions: ["assignDeploymentFlags"],
          },
          onError: {
            target: "idle",
            actions: ["assignGetDeploymentFlagsError"],
          },
        },
      },
      loaded: {
        type: "final",
      },
    },
  },
  {
    services: {
      getDeploymentFlags,
    },
    actions: {
      assignDeploymentFlags: assign({
        deploymentFlags: (_, { data }) => data,
      }),
      assignGetDeploymentFlagsError: assign({
        getDeploymentFlagsError: (_, { data }) => data,
      }),
    },
  },
)
