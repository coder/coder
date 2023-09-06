import { assign, createMachine } from "xstate";
import * as API from "../../api/api";
import { ProvisionerJobLog, WorkspaceBuild } from "../../api/typesGenerated";

type LogsContext = {
  // Build
  username: string;
  workspaceName: string;
  buildNumber: number;
  buildId: string;
  // Used to reference logs before + after.
  timeCursor: Date;
  build?: WorkspaceBuild;
  getBuildError?: unknown;
  // Logs
  logs?: ProvisionerJobLog[];
};

type LogsEvent =
  | {
      type: "ADD_LOG";
      log: ProvisionerJobLog;
    }
  | {
      type: "BUILD_DONE";
    }
  | {
      type: "RESET";
      buildNumber: number;
      timeCursor: Date;
    };

export const workspaceBuildMachine = createMachine(
  {
    id: "workspaceBuildState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceBuildXService.typegen").Typegen0,
    schema: {
      context: {} as LogsContext,
      events: {} as LogsEvent,
      services: {} as {
        getWorkspaceBuild: {
          data: WorkspaceBuild;
        };
        getLogs: {
          data: ProvisionerJobLog[];
        };
      },
    },
    initial: "gettingBuild",
    on: {
      RESET: {
        target: "gettingBuild",
        actions: ["resetContext"],
      },
    },
    states: {
      gettingBuild: {
        entry: "clearGetBuildError",
        invoke: {
          src: "getWorkspaceBuild",
          onDone: {
            target: "logs",
            actions: ["assignBuild", "assignBuildId"],
          },
          onError: {
            target: "idle",
            actions: "assignGetBuildError",
          },
        },
      },
      idle: {},
      logs: {
        initial: "gettingExistentLogs",
        states: {
          gettingExistentLogs: {
            invoke: {
              id: "getLogs",
              src: "getLogs",
              onDone: {
                actions: ["assignLogs"],
                target: "watchingLogs",
              },
            },
          },
          watchingLogs: {
            id: "watchingLogs",
            invoke: {
              id: "streamWorkspaceBuildLogs",
              src: "streamWorkspaceBuildLogs",
            },
            on: {
              ADD_LOG: {
                actions: "addLog",
              },
              BUILD_DONE: {
                target: "loaded",
              },
            },
          },
          loaded: {
            type: "final",
          },
        },
      },
    },
  },
  {
    actions: {
      resetContext: assign({
        buildNumber: (_, event) => event.buildNumber,
        timeCursor: (_, event) => event.timeCursor,
        logs: undefined,
      }),
      // Build ID
      assignBuildId: assign({
        buildId: (_, event) => event.data.id,
      }),
      // Build
      assignBuild: assign({
        build: (_, event) => event.data,
      }),
      assignGetBuildError: assign({
        getBuildError: (_, event) => event.data,
      }),
      clearGetBuildError: assign({
        getBuildError: (_) => undefined,
      }),
      // Logs
      assignLogs: assign({
        logs: (_, event) => event.data,
      }),
      addLog: assign({
        logs: (context, event) => {
          const previousLogs = context.logs ?? [];
          return [...previousLogs, event.log];
        },
      }),
    },
    services: {
      getWorkspaceBuild: (ctx) =>
        API.getWorkspaceBuildByNumber(
          ctx.username,
          ctx.workspaceName,
          ctx.buildNumber,
        ),
      getLogs: async (ctx) =>
        API.getWorkspaceBuildLogs(ctx.buildId, ctx.timeCursor),
      streamWorkspaceBuildLogs: (ctx) => async (callback) => {
        if (!ctx.logs) {
          throw new Error("logs must be set");
        }
        const after =
          ctx.logs.length > 0 ? ctx.logs[ctx.logs.length - 1].id : undefined;
        const socket = API.watchBuildLogsByBuildId(ctx.buildId, {
          after,
          onMessage: (log) => {
            callback({ type: "ADD_LOG", log });
          },
          onDone: () => {
            callback({ type: "BUILD_DONE" });
          },
          onError: (err) => {
            console.error(err);
          },
        });
        return () => {
          socket.close();
        };
      },
    },
  },
);
