import { DAUsResponse } from "./../../api/typesGenerated";
import {
  getDeploymentValues,
  getDeploymentDAUs,
  DeploymentConfig,
} from "api/api";
import { createMachine, assign } from "xstate";

export const deploymentConfigMachine = createMachine(
  {
    id: "deploymentConfigMachine",
    predictableActionArguments: true,

    schema: {
      context: {} as {
        deploymentValues?: DeploymentConfig;
        getDeploymentValuesError?: unknown;
        deploymentDAUs?: DAUsResponse;
        getDeploymentDAUsError?: unknown;
      },
      events: {} as { type: "LOAD" },
      services: {} as {
        getDeploymentValues: {
          data: DeploymentConfig;
        };
        getDeploymentDAUs: {
          data: DAUsResponse;
        };
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
      getDeploymentDAUs: async () => {
        return getDeploymentDAUs();
      },
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
);
