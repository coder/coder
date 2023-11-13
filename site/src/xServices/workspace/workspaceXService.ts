import { assign, createMachine } from "xstate";
import * as API from "api/api";
import * as TypesGen from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";

export interface WorkspaceContext {
  // Initial data
  orgId: string;
  username: string;
  workspaceName: string;
  error?: unknown;
  // our server side events instance
  eventSource?: EventSource;
  workspace?: TypesGen.Workspace;
  template?: TypesGen.Template;
  permissions?: Permissions;
  build?: TypesGen.WorkspaceBuild;
  // Builds
  builds?: TypesGen.WorkspaceBuild[];
  getBuildsError?: unknown;
  // error creating a new WorkspaceBuild
  buildError?: unknown;
  cancellationMessage?: TypesGen.Response;
  cancellationError?: unknown;
  // debug
  createBuildLogLevel?: TypesGen.CreateWorkspaceBuildRequest["log_level"];
  // Change version
  templateVersionIdToChange?: TypesGen.TemplateVersion["id"];
}

export type WorkspaceEvent =
  | { type: "START"; buildParameters?: TypesGen.WorkspaceBuildParameter[] }
  | { type: "CANCEL" }
  | { type: "RETRY_BUILD" };

export const workspaceMachine = createMachine(
  {
    id: "workspaceState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceXService.typegen").Typegen0,
    schema: {
      context: {} as WorkspaceContext,
      events: {} as WorkspaceEvent,
      services: {} as {
        startWorkspace: {
          data: TypesGen.WorkspaceBuild;
        };
        stopWorkspace: {
          data: TypesGen.WorkspaceBuild;
        };
        cancelWorkspace: {
          data: TypesGen.Response;
        };
        activateWorkspace: {
          data: TypesGen.Response;
        };
      },
    },
    initial: "ready",
    states: {
      ready: {
        type: "parallel",
        states: {
          build: {
            initial: "idle",
            states: {
              idle: {
                on: {
                  START: "requestingStart",
                  CANCEL: "requestingCancel",
                  RETRY_BUILD: [
                    {
                      target: "requestingStart",
                      cond: "lastBuildWasStarting",
                      actions: ["enableDebugMode"],
                    },
                  ],
                },
              },
              requestingStart: {
                entry: ["clearBuildError"],
                invoke: {
                  src: "startWorkspace",
                  id: "startWorkspace",
                  onDone: [
                    {
                      actions: ["assignBuild", "disableDebugMode"],
                      target: "idle",
                    },
                  ],
                  onError: [
                    {
                      actions: "assignBuildError",
                      target: "idle",
                    },
                  ],
                },
              },
              requestingCancel: {
                entry: ["clearCancellationMessage", "clearCancellationError"],
                invoke: {
                  src: "cancelWorkspace",
                  id: "cancelWorkspace",
                  onDone: [
                    {
                      actions: [
                        "assignCancellationMessage",
                        "displayCancellationMessage",
                      ],
                      target: "idle",
                    },
                  ],
                  onError: [
                    {
                      actions: "assignCancellationError",
                      target: "idle",
                    },
                  ],
                },
              },
            },
          },
        },
      },
      error: {
        type: "final",
      },
    },
  },
  {
    actions: {
      assignBuild: assign({
        build: (_, event) => event.data,
      }),
      assignBuildError: assign({
        buildError: (_, event) => event.data,
      }),
      clearBuildError: assign({
        buildError: (_) => undefined,
      }),
      assignCancellationMessage: assign({
        cancellationMessage: (_, event) => event.data,
      }),
      clearCancellationMessage: assign({
        cancellationMessage: (_) => undefined,
      }),
      displayCancellationMessage: (context) => {
        if (context.cancellationMessage) {
          displaySuccess(context.cancellationMessage.message);
        }
      },
      assignCancellationError: assign({
        cancellationError: (_, event) => event.data,
      }),
      clearCancellationError: assign({
        cancellationError: (_) => undefined,
      }),

      // Debug mode when build fails
      enableDebugMode: assign({ createBuildLogLevel: (_) => "debug" as const }),
      disableDebugMode: assign({ createBuildLogLevel: (_) => undefined }),
    },
    guards: {
      lastBuildWasStarting: ({ workspace }) => {
        return workspace?.latest_build.transition === "start";
      },
    },
    services: {
      startWorkspace: (context, data) => async () => {
        if (context.workspace) {
          const startWorkspacePromise = await API.startWorkspace(
            context.workspace.id,
            context.workspace.latest_build.template_version_id,
            context.createBuildLogLevel,
            "buildParameters" in data ? data.buildParameters : undefined,
          );

          return startWorkspacePromise;
        } else {
          throw Error("Cannot start workspace without workspace id");
        }
      },
      cancelWorkspace: (context) => async () => {
        if (context.workspace) {
          const cancelWorkspacePromise = await API.cancelWorkspaceBuild(
            context.workspace.latest_build.id,
          );

          return cancelWorkspacePromise;
        } else {
          throw Error("Cannot cancel workspace without build id");
        }
      },
    },
  },
);
