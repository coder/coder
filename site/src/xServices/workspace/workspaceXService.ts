import { getErrorMessage } from "api/errors";
import { assign, createMachine } from "xstate";
import * as API from "api/api";
import * as TypesGen from "api/typesGenerated";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";

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
  | { type: "REFRESH_WORKSPACE"; data: TypesGen.ServerSentEvent["data"] }
  | { type: "START"; buildParameters?: TypesGen.WorkspaceBuildParameter[] }
  | { type: "STOP" }
  | { type: "ASK_DELETE" }
  | { type: "DELETE" }
  | { type: "CANCEL_DELETE" }
  | { type: "CANCEL" }
  | {
      type: "REFRESH_TIMELINE";
    }
  | { type: "RETRY_BUILD" }
  | { type: "ACTIVATE" };

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
        deleteWorkspace: {
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
        on: {
          REFRESH_TIMELINE: {
            actions: ["refreshBuilds"],
          },
        },
        states: {
          build: {
            initial: "idle",
            states: {
              idle: {
                on: {
                  START: "requestingStart",
                  STOP: "requestingStop",
                  ASK_DELETE: "askingDelete",
                  CANCEL: "requestingCancel",
                  RETRY_BUILD: [
                    {
                      target: "requestingStart",
                      cond: "lastBuildWasStarting",
                      actions: ["enableDebugMode"],
                    },
                    {
                      target: "requestingStop",
                      cond: "lastBuildWasStopping",
                      actions: ["enableDebugMode"],
                    },
                    {
                      target: "requestingDelete",
                      cond: "lastBuildWasDeleting",
                      actions: ["enableDebugMode"],
                    },
                  ],
                  ACTIVATE: "requestingActivate",
                },
              },
              askingDelete: {
                on: {
                  DELETE: {
                    target: "requestingDelete",
                  },
                  CANCEL_DELETE: {
                    target: "idle",
                  },
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
              requestingStop: {
                entry: ["clearBuildError"],
                invoke: {
                  src: "stopWorkspace",
                  id: "stopWorkspace",
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
              requestingDelete: {
                entry: ["clearBuildError"],
                invoke: {
                  src: "deleteWorkspace",
                  id: "deleteWorkspace",
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
              requestingActivate: {
                entry: ["clearBuildError"],
                invoke: {
                  src: "activateWorkspace",
                  id: "activateWorkspace",
                  onDone: "idle",
                  onError: {
                    target: "idle",
                    actions: ["displayActivateError"],
                  },
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
      displayActivateError: (_, { data }) => {
        const message = getErrorMessage(data, "Error activate workspace.");
        displayError(message);
      },

      // Debug mode when build fails
      enableDebugMode: assign({ createBuildLogLevel: (_) => "debug" as const }),
      disableDebugMode: assign({ createBuildLogLevel: (_) => undefined }),
    },
    guards: {
      lastBuildWasStarting: ({ workspace }) => {
        return workspace?.latest_build.transition === "start";
      },
      lastBuildWasStopping: ({ workspace }) => {
        return workspace?.latest_build.transition === "stop";
      },
      lastBuildWasDeleting: ({ workspace }) => {
        return workspace?.latest_build.transition === "delete";
      },
    },
    services: {
      startWorkspace: (context, data) => async (send) => {
        if (context.workspace) {
          const startWorkspacePromise = await API.startWorkspace(
            context.workspace.id,
            context.workspace.latest_build.template_version_id,
            context.createBuildLogLevel,
            "buildParameters" in data ? data.buildParameters : undefined,
          );
          send({ type: "REFRESH_TIMELINE" });
          return startWorkspacePromise;
        } else {
          throw Error("Cannot start workspace without workspace id");
        }
      },
      stopWorkspace: (context) => async (send) => {
        if (context.workspace) {
          const stopWorkspacePromise = await API.stopWorkspace(
            context.workspace.id,
            context.createBuildLogLevel,
          );
          send({ type: "REFRESH_TIMELINE" });
          return stopWorkspacePromise;
        } else {
          throw Error("Cannot stop workspace without workspace id");
        }
      },
      deleteWorkspace: (context) => async (send) => {
        if (context.workspace) {
          const deleteWorkspacePromise = await API.deleteWorkspace(
            context.workspace.id,
            context.createBuildLogLevel,
          );
          send({ type: "REFRESH_TIMELINE" });
          return deleteWorkspacePromise;
        } else {
          throw Error("Cannot delete workspace without workspace id");
        }
      },
      cancelWorkspace: (context) => async (send) => {
        if (context.workspace) {
          const cancelWorkspacePromise = await API.cancelWorkspaceBuild(
            context.workspace.latest_build.id,
          );
          send({ type: "REFRESH_TIMELINE" });
          return cancelWorkspacePromise;
        } else {
          throw Error("Cannot cancel workspace without build id");
        }
      },
      activateWorkspace: (context) => async (send) => {
        if (context.workspace) {
          const activateWorkspacePromise = await API.updateWorkspaceDormancy(
            context.workspace.id,
            false,
          );
          send({ type: "REFRESH_WORKSPACE", data: activateWorkspacePromise });
          return activateWorkspacePromise;
        } else {
          throw Error("Cannot activate workspace without workspace id");
        }
      },
    },
  },
);
