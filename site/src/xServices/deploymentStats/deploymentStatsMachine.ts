import { getDeploymentStats } from "api/api"
import { DeploymentStats } from "api/typesGenerated"
import { assign, createMachine } from "xstate"

export const deploymentStatsMachine = createMachine(
  {
    id: "deploymentStatsMachine",
    predictableActionArguments: true,

    schema: {
      context: {} as {
        deploymentStats?: DeploymentStats
        getDeploymentStatsError?: unknown
      },
      events: {} as { type: "RELOAD" },
      services: {} as {
        getDeploymentStats: {
          data: DeploymentStats
        }
      },
    },
    tsTypes: {} as import("./deploymentStatsMachine.typegen").Typegen0,
    initial: "stats",
    states: {
      stats: {
        invoke: {
          src: "getDeploymentStats",
          onDone: {
            target: "idle",
            actions: ["assignDeploymentStats"],
          },
          onError: {
            target: "idle",
            actions: ["assignDeploymentStatsError"],
          },
        },
        tags: "loading",
      },
      idle: {
        on: {
          RELOAD: "stats",
        },
      },
    },
  },
  {
    services: {
      getDeploymentStats: getDeploymentStats,
    },
    actions: {
      assignDeploymentStats: assign({
        deploymentStats: (_, { data }) => data,
      }),
      assignDeploymentStatsError: assign({
        getDeploymentStatsError: (_, { data }) => data,
      }),
    },
  },
)
