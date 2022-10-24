import { getDeploymentConfig } from "api/api"
import { DeploymentConfig } from "api/typesGenerated"
import { createMachine, assign } from "xstate"

export const deploymentConfigMachine = createMachine(
  {
    id: "deploymentConfigMachine",
    predictableActionArguments: true,
    initial: "idle",
    schema: {
      context: {} as {
        deploymentConfig?: DeploymentConfig
        getDeploymentConfigError?: unknown
      },
      events: {} as { type: "LOAD" },
      services: {} as {
        getDeploymentConfig: {
          data: DeploymentConfig
        }
      },
    },
    tsTypes: {} as import("./deploymentConfigMachine.typegen").Typegen0,
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
          src: "getDeploymentConfig",
          onDone: {
            target: "loaded",
            actions: ["assignDeploymentConfig"],
          },
          onError: {
            target: "idle",
            actions: ["assignGetDeploymentConfigError"],
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
      getDeploymentConfig: getDeploymentConfig,
    },
    actions: {
      assignDeploymentConfig: assign({
        deploymentConfig: (_, { data }) => data,
      }),
      assignGetDeploymentConfigError: assign({
        getDeploymentConfigError: (_, { data }) => data,
      }),
    },
  },
)
