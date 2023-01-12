import { getDeploymentConfig } from "api/api"
import { DeploymentConfig } from "api/typesGenerated"
import { createMachine, assign } from "xstate"

export const deploymentConfigMachine = createMachine(
  {
    id: "deploymentConfigMachine",
    predictableActionArguments: true,

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
    initial: "loading",
    states: {
      loading: {
        invoke: {
          src: "getDeploymentConfig",
          onDone: {
            target: "done",
            actions: ["assignDeploymentConfig"],
          },
          onError: {
            target: "done",
            actions: ["assignGetDeploymentConfigError"],
          },
        },
      },
      done: {
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
