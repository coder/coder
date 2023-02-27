import { DeploymentDAUsResponse } from "./../../api/typesGenerated"
import { getDeploymentConfig, getDeploymentDAUs } from "api/api"
import { createMachine, assign } from "xstate"
import { DeploymentConfigAndOptions } from "api/types"

export const deploymentConfigMachine = createMachine(
  {
    id: "deploymentConfigMachine",
    predictableActionArguments: true,

    schema: {
      context: {} as {
        deploymentConfig?: DeploymentConfigAndOptions
        getDeploymentConfigError?: unknown
        deploymentDAUs?: DeploymentDAUsResponse
        getDeploymentDAUsError?: unknown
      },
      events: {} as { type: "LOAD" },
      services: {} as {
        getDeploymentConfig: {
          data: DeploymentConfigAndOptions
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
          src: "getDeploymentConfig",
          onDone: {
            target: "daus",
            actions: ["assignDeploymentConfig"],
          },
          onError: {
            target: "daus",
            actions: ["assignGetDeploymentConfigError"],
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
      getDeploymentConfig: getDeploymentConfig,
      getDeploymentDAUs: getDeploymentDAUs,
    },
    actions: {
      assignDeploymentConfig: assign({
        deploymentConfig: (_, { data }) => data,
      }),
      assignGetDeploymentConfigError: assign({
        getDeploymentConfigError: (_, { data }) => data,
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
