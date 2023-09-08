import { getErrorMessage } from "api/errors";
import dayjs from "dayjs";
import { workspaceScheduleBannerMachine } from "xServices/workspaceSchedule/workspaceScheduleBannerXService";
import { assign, createMachine, send } from "xstate";
import * as API from "../../api/api";
import * as Types from "../../api/types";
import * as TypesGen from "../../api/typesGenerated";
import {
  displayError,
  displaySuccess,
} from "../../components/GlobalSnackbar/utils";

const latestBuild = (builds: TypesGen.WorkspaceBuild[]) => {
  // Cloning builds to not change the origin object with the sort()
  return [...builds].sort((a, b) => {
    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
  })[0];
};

const moreBuildsAvailable = (
  context: WorkspaceContext,
  event: {
    type: "REFRESH_TIMELINE";
    checkRefresh?: boolean;
    data?: TypesGen.ServerSentEvent["data"];
  },
) => {
  // No need to refresh the timeline if it is not loaded
  if (!context.builds) {
    return false;
  }

  if (!event.checkRefresh) {
    return true;
  }

  // After we refresh a workspace, we want to check if the latest
  // build was updated before refreshing the timeline so as to not over fetch the builds
  const latestBuildInTimeline = latestBuild(context.builds);
  return (
    event.data.latest_build.updated_at !== latestBuildInTimeline.updated_at
  );
};

type Permissions = Record<keyof ReturnType<typeof permissionsToCheck>, boolean>;

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
  templateVersion?: TypesGen.TemplateVersion;
  deploymentValues?: TypesGen.DeploymentValues;
  build?: TypesGen.WorkspaceBuild;
  // Builds
  builds?: TypesGen.WorkspaceBuild[];
  getBuildsError?: unknown;
  missedParameters?: TypesGen.TemplateVersionParameter[];
  // error creating a new WorkspaceBuild
  buildError?: unknown;
  cancellationMessage?: Types.Message;
  cancellationError?: unknown;
  // debug
  createBuildLogLevel?: TypesGen.CreateWorkspaceBuildRequest["log_level"];
  // SSH Config
  sshPrefix?: string;
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
  | { type: "UPDATE"; buildParameters?: TypesGen.WorkspaceBuildParameter[] }
  | {
      type: "CHANGE_VERSION";
      templateVersionId: TypesGen.TemplateVersion["id"];
      buildParameters?: TypesGen.WorkspaceBuildParameter[];
    }
  | { type: "CANCEL" }
  | {
      type: "REFRESH_TIMELINE";
      checkRefresh?: boolean;
      data?: TypesGen.ServerSentEvent["data"];
    }
  | { type: "EVENT_SOURCE_ERROR"; error: unknown }
  | { type: "INCREASE_DEADLINE"; hours: number }
  | { type: "DECREASE_DEADLINE"; hours: number }
  | { type: "RETRY_BUILD" }
  | { type: "ACTIVATE" };

export const checks = {
  readWorkspace: "readWorkspace",
  updateWorkspace: "updateWorkspace",
  updateTemplate: "updateTemplate",
  viewDeploymentValues: "viewDeploymentValues",
} as const;

const permissionsToCheck = (
  workspace: TypesGen.Workspace,
  template: TypesGen.Template,
) =>
  ({
    [checks.readWorkspace]: {
      object: {
        resource_type: "workspace",
        resource_id: workspace.id,
        owner_id: workspace.owner_id,
      },
      action: "read",
    },
    [checks.updateWorkspace]: {
      object: {
        resource_type: "workspace",
        resource_id: workspace.id,
        owner_id: workspace.owner_id,
      },
      action: "update",
    },
    [checks.updateTemplate]: {
      object: {
        resource_type: "template",
        resource_id: template.id,
      },
      action: "update",
    },
    [checks.viewDeploymentValues]: {
      object: {
        resource_type: "deployment_config",
      },
      action: "read",
    },
  }) as const;

export const workspaceMachine = createMachine(
  {
    id: "workspaceState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceXService.typegen").Typegen0,
    schema: {
      context: {} as WorkspaceContext,
      events: {} as WorkspaceEvent,
      services: {} as {
        loadInitialWorkspaceData: {
          data: Awaited<ReturnType<typeof loadInitialWorkspaceData>>;
        };
        updateWorkspace: {
          data: TypesGen.WorkspaceBuild;
        };
        changeWorkspaceVersion: {
          data: TypesGen.WorkspaceBuild;
        };
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
          data: Types.Message;
        };
        activateWorkspace: {
          data: Types.Message;
        };
        listening: {
          data: TypesGen.ServerSentEvent;
        };
        getBuilds: {
          data: TypesGen.WorkspaceBuild[];
        };
        getSSHPrefix: {
          data: TypesGen.SSHConfigResponse;
        };
      },
    },
    initial: "loadInitialData",
    states: {
      loadInitialData: {
        entry: ["clearContext"],
        invoke: {
          src: "loadInitialWorkspaceData",
          id: "loadInitialWorkspaceData",
          onDone: [{ target: "ready", actions: ["assignInitialData"] }],
          onError: [
            {
              actions: "assignError",
              target: "error",
            },
          ],
        },
      },
      ready: {
        type: "parallel",
        states: {
          listening: {
            initial: "gettingEvents",
            states: {
              gettingEvents: {
                entry: ["initializeEventSource"],
                exit: "closeEventSource",
                invoke: {
                  src: "listening",
                  id: "listening",
                },
                on: {
                  REFRESH_WORKSPACE: {
                    actions: ["refreshWorkspace"],
                  },
                  EVENT_SOURCE_ERROR: {
                    target: "error",
                  },
                },
              },
              error: {
                entry: "logWatchWorkspaceWarning",
                after: {
                  "2000": {
                    target: "gettingEvents",
                  },
                },
              },
            },
          },
          build: {
            initial: "idle",
            states: {
              idle: {
                on: {
                  START: "requestingStart",
                  STOP: "requestingStop",
                  ASK_DELETE: "askingDelete",
                  UPDATE: "requestingUpdate",
                  CHANGE_VERSION: {
                    target: "requestingChangeVersion",
                    actions: ["assignTemplateVersionIdToChange"],
                  },
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
              requestingUpdate: {
                entry: ["clearBuildError"],
                invoke: {
                  src: "updateWorkspace",
                  onDone: {
                    target: "idle",
                    actions: ["assignBuild"],
                  },
                  onError: [
                    {
                      target: "askingForMissedBuildParameters",
                      cond: "isMissingBuildParameterError",
                      actions: ["assignMissedParameters"],
                    },
                    {
                      target: "idle",
                      actions: ["assignBuildError"],
                    },
                  ],
                },
              },
              requestingChangeVersion: {
                entry: ["clearBuildError"],
                invoke: {
                  src: "changeWorkspaceVersion",
                  onDone: {
                    target: "idle",
                    actions: ["assignBuild", "clearTemplateVersionIdToChange"],
                  },
                  onError: [
                    {
                      target: "askingForMissedBuildParameters",
                      cond: "isMissingBuildParameterError",
                      actions: ["assignMissedParameters"],
                    },
                    {
                      target: "idle",
                      actions: ["assignBuildError"],
                    },
                  ],
                },
              },
              askingForMissedBuildParameters: {
                on: {
                  CANCEL: "idle",
                  UPDATE: [
                    {
                      target: "requestingChangeVersion",
                      cond: "isChangingVersion",
                    },
                    { target: "requestingUpdate" },
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
          timeline: {
            initial: "gettingBuilds",
            states: {
              gettingBuilds: {
                invoke: {
                  src: "getBuilds",
                  onDone: [
                    {
                      actions: ["assignBuilds", "clearGetBuildsError"],
                      target: "loadedBuilds",
                    },
                  ],
                  onError: [
                    {
                      actions: "assignGetBuildsError",
                      target: "loadedBuilds",
                    },
                  ],
                },
              },
              loadedBuilds: {
                on: {
                  REFRESH_TIMELINE: {
                    target: "#workspaceState.ready.timeline.gettingBuilds",
                    cond: "moreBuildsAvailable",
                  },
                },
              },
            },
          },
          sshConfig: {
            initial: "gettingSshConfig",
            states: {
              gettingSshConfig: {
                invoke: {
                  src: "getSSHPrefix",
                  onDone: {
                    target: "success",
                    actions: ["assignSSHPrefix"],
                  },
                  onError: {
                    target: "error",
                    actions: ["displaySSHPrefixError"],
                  },
                },
              },
              error: {
                type: "final",
              },
              success: {
                type: "final",
              },
            },
          },
          schedule: {
            invoke: {
              id: "scheduleBannerMachine",
              src: "scheduleBannerMachine",
              data: {
                workspace: (context: WorkspaceContext) => context.workspace,
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
      // Clear data about an old workspace when looking at a new one
      clearContext: () =>
        assign({
          workspace: undefined,
          template: undefined,
          build: undefined,
          permissions: undefined,
          eventSource: undefined,
        }),
      assignInitialData: assign({
        workspace: (_, event) => event.data.workspace,
        template: (_, event) => event.data.template,
        templateVersion: (_, event) => event.data.templateVersion,
        permissions: (_, event) => event.data.permissions as Permissions,
        deploymentValues: (_, event) => event.data.deploymentValues,
      }),
      assignError: assign({
        error: (_, event) => event.data,
      }),
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
      // SSE related actions
      // open a new EventSource so we can stream SSE
      initializeEventSource: assign({
        eventSource: (context) =>
          context.workspace && API.watchWorkspace(context.workspace.id),
      }),
      closeEventSource: (context) =>
        context.eventSource && context.eventSource.close(),
      refreshWorkspace: assign({
        workspace: (_, event) => event.data,
      }),
      logWatchWorkspaceWarning: (_, event) => {
        console.error("Watch workspace error:", event);
      },
      // Timeline
      assignBuilds: assign({
        builds: (_, event) => event.data,
      }),
      assignGetBuildsError: assign({
        getBuildsError: (_, event) => event.data,
      }),
      clearGetBuildsError: assign({
        getBuildsError: (_) => undefined,
      }),
      // SSH
      assignSSHPrefix: assign({
        sshPrefix: (_, { data }) => data.hostname_prefix,
      }),
      displaySSHPrefixError: (_, { data }) => {
        const message = getErrorMessage(
          data,
          "Error getting the deployment ssh configuration.",
        );
        displayError(message);
      },
      displayActivateError: (_, { data }) => {
        const message = getErrorMessage(data, "Error activate workspace.");
        displayError(message);
      },
      assignMissedParameters: assign({
        missedParameters: (_, { data }) => {
          if (!(data instanceof API.MissingBuildParameters)) {
            throw new Error("data is not a MissingBuildParameters error");
          }
          return data.parameters;
        },
      }),
      // Debug mode when build fails
      enableDebugMode: assign({ createBuildLogLevel: (_) => "debug" as const }),
      disableDebugMode: assign({ createBuildLogLevel: (_) => undefined }),
      // Change version
      assignTemplateVersionIdToChange: assign({
        templateVersionIdToChange: (_, { templateVersionId }) =>
          templateVersionId,
      }),
      clearTemplateVersionIdToChange: assign({
        templateVersionIdToChange: (_) => undefined,
      }),
    },
    guards: {
      moreBuildsAvailable,
      isMissingBuildParameterError: (_, { data }) => {
        return data instanceof API.MissingBuildParameters;
      },
      lastBuildWasStarting: ({ workspace }) => {
        return workspace?.latest_build.transition === "start";
      },
      lastBuildWasStopping: ({ workspace }) => {
        return workspace?.latest_build.transition === "stop";
      },
      lastBuildWasDeleting: ({ workspace }) => {
        return workspace?.latest_build.transition === "delete";
      },
      isChangingVersion: ({ templateVersionIdToChange }) =>
        Boolean(templateVersionIdToChange),
    },
    services: {
      loadInitialWorkspaceData,
      updateWorkspace:
        ({ workspace }, { buildParameters }) =>
        async (send) => {
          if (!workspace) {
            throw new Error("Workspace is not set");
          }
          const build = await API.updateWorkspace(workspace, buildParameters);
          send({ type: "REFRESH_TIMELINE" });
          return build;
        },
      changeWorkspaceVersion:
        ({ workspace, templateVersionIdToChange }, { buildParameters }) =>
        async (send) => {
          if (!workspace) {
            throw new Error("Workspace is not set");
          }
          if (!templateVersionIdToChange) {
            throw new Error("Template version id to change is not set");
          }
          const build = await API.changeWorkspaceVersion(
            workspace,
            templateVersionIdToChange,
            buildParameters,
          );
          send({ type: "REFRESH_TIMELINE" });
          return build;
        },
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
      deleteWorkspace: async (context) => {
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
      listening: (context) => (send) => {
        if (!context.eventSource) {
          send({ type: "EVENT_SOURCE_ERROR", error: "error initializing sse" });
          return;
        }

        context.eventSource.addEventListener("data", (event) => {
          // refresh our workspace with each SSE
          send({ type: "REFRESH_WORKSPACE", data: JSON.parse(event.data) });
          // refresh our timeline
          send({
            type: "REFRESH_TIMELINE",
            checkRefresh: true,
            data: JSON.parse(event.data),
          });
          // refresh
        });

        // handle any error events returned by our sse
        context.eventSource.addEventListener("error", (event) => {
          send({ type: "EVENT_SOURCE_ERROR", error: event });
        });

        // handle any sse implementation exceptions
        context.eventSource.onerror = () => {
          send({ type: "EVENT_SOURCE_ERROR", error: "sse error" });
        };

        return () => {
          context.eventSource?.close();
        };
      },
      getBuilds: async (context) => {
        if (context.workspace) {
          // For now, we only retrieve the last month of builds to minimize
          // page bloat. We should add pagination in the future.
          return await API.getWorkspaceBuilds(
            context.workspace.id,
            dayjs().add(-30, "day").toDate(),
          );
        } else {
          throw Error("Cannot get builds without id");
        }
      },
      scheduleBannerMachine: workspaceScheduleBannerMachine,
      getSSHPrefix: async () => {
        return API.getDeploymentSSHConfig();
      },
    },
  },
);

async function loadInitialWorkspaceData({
  orgId,
  username,
  workspaceName,
}: WorkspaceContext) {
  const workspace = await API.getWorkspaceByOwnerAndName(
    username,
    workspaceName,
    {
      include_deleted: true,
    },
  );
  const template = await API.getTemplateByName(orgId, workspace.template_name);
  const [templateVersion, permissions] = await Promise.all([
    API.getTemplateVersion(template.active_version_id),
    API.checkAuthorization({
      checks: permissionsToCheck(workspace, template),
    }),
  ]);

  const canViewDeploymentValues = Boolean(
    (permissions as Permissions)?.viewDeploymentValues,
  );
  const deploymentValues = canViewDeploymentValues
    ? (await API.getDeploymentValues())?.config
    : undefined;
  return {
    workspace,
    template,
    templateVersion,
    permissions,
    deploymentValues,
  };
}
