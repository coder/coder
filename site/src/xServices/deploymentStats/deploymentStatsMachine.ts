import { getDeploymentStats } from "api/api";
import { DeploymentStats } from "api/typesGenerated";
import { assign, createMachine } from "xstate";

export const deploymentStatsMachine = createMachine(
  {
    /** @xstate-layout N4IgpgJg5mDOIC5QTABwDYHsCeBbMAdgC4DKRAhkbALLkDGAFgJYFgB0sFVAxBJq2xYA3TAGt2KDDnzEylGvWYDO8hMMx1KTfgG0ADAF19BxKFSZYTItoKmQAD0QBWAMwuANCGyIATAHYATgBfIM9JLDxCUi4FRhZ2FR4wACdkzGS2DEoAM3TcNnDpKLkqWjjlGLUCEU1rXUNjO3NLOtskB0QAvU9vBAAWVxCwtAiZaPkypXYmCHQwbgAlAFEAGQB5AEEAEUb25qsbO0cEAEY9AA4exDOANhDQkAJMFHh2wsjZGMn4posD-iOiD6Piup3ObCcQxA7zGJViUw4MV+LUO7WOfT03S81xOLihMOKX0U8UEszAyP+bVAxxc5z6oJ8AT6EPuQSAA */
    id: "deploymentStatsMachine",
    predictableActionArguments: true,

    schema: {
      context: {} as {
        deploymentStats?: DeploymentStats;
        getDeploymentStatsError?: unknown;
      },
      events: {} as { type: "RELOAD" },
      services: {} as {
        getDeploymentStats: {
          data: DeploymentStats;
        };
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
);
