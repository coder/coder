import { DeploymentDAUsResponse } from "./../../api/typesGenerated"
import { getDeploymentValues, getDeploymentDAUs } from "api/api"
import { createMachine, assign } from "xstate"
import { DeploymentConfig } from "api/types"

export const deploymentConfigMachine = createMachine(
  {
    id: "deploymentConfigMachine",
    predictableActionArguments: true,

    schema: {
      context: {} as {
        deploymentValues?: DeploymentConfig
        getDeploymentValuesError?: unknown
        deploymentDAUs?: DeploymentDAUsResponse
        getDeploymentDAUsError?: unknown
      },
      events: {} as { type: "LOAD" },
      services: {} as {
        getDeploymentValues: {
          data: DeploymentConfig
        }
        getDeploymentDAUs: {
          data: DeploymentDAUsResponse
        }
      },
    },
    tsTypes: {} as import("./deploymentConfigMachine.typegen").Typegen0,
    initial: "config",
    states: {
      config: {
        invoke: {
          src: "getDeploymentValues",
          onDone: {
            target: "daus",
            actions: ["assignDeploymentValues"],
          },
          onError: {
            target: "daus",
            actions: ["assignGetDeploymentValuesError"],
          },
        },
        tags: "loading",
      },
      daus: {
        invoke: {
          src: "getDeploymentDAUs",
          onDone: {
            target: "done",
            actions: ["assignDeploymentDAUs"],
          },
          onError: {
            target: "done",
            actions: ["assignGetDeploymentDAUsError"],
          },
        },
        tags: "loading",
      },
      done: {
        type: "final",
      },
    },
  },
  {
    services: {
      getDeploymentValues: getDeploymentValues,
      getDeploymentDAUs: getDeploymentDAUs,
    },
    actions: {
      assignDeploymentValues: assign({
        deploymentValues: (_, { data }) => data,
      }),
      assignGetDeploymentValuesError: assign({
        getDeploymentValuesError: (_, { data }) => data,
      }),
      assignDeploymentDAUs: assign({
        deploymentDAUs: (_, { data }) => data,
      }),
      assignGetDeploymentDAUsError: assign({
        getDeploymentDAUsError: (_, { data }) => data,
      }),
    },
  },
)
