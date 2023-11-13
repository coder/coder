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

export type WorkspaceEvent = { type: "CANCEL" } | { type: "RETRY_BUILD" };

export const workspaceMachine = createMachine(
  {
    id: "workspaceState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceXService.typegen").Typegen0,
    schema: {
      context: {} as WorkspaceContext,
      events: {} as WorkspaceEvent,
      services: {} as {
        cancelWorkspace: {
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
                  CANCEL: "requestingCancel",
                  RETRY_BUILD: [],
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
    },
    services: {
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
