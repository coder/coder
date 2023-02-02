import { getErrorMessage } from "api/errors"
import dayjs from "dayjs"
import { workspaceScheduleBannerMachine } from "xServices/workspaceSchedule/workspaceScheduleBannerXService"
import { assign, createMachine, send } from "xstate"
import * as API from "../../api/api"
import * as Types from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"
import {
  displayError,
  displaySuccess,
} from "../../components/GlobalSnackbar/utils"

const latestBuild = (builds: TypesGen.WorkspaceBuild[]) => {
  // Cloning builds to not change the origin object with the sort()
  return [...builds].sort((a, b) => {
    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
  })[0]
}

const moreBuildsAvailable = (
  context: WorkspaceContext,
  event: {
    type: "REFRESH_TIMELINE"
    checkRefresh?: boolean
    data?: TypesGen.ServerSentEvent["data"]
  },
) => {
  // No need to refresh the timeline if it is not loaded
  if (!context.builds) {
    return false
  }

  if (!event.checkRefresh) {
    return true
  }

  // After we refresh a workspace, we want to check if the latest
  // build was updated before refreshing the timeline so as to not over fetch the builds
  const latestBuildInTimeline = latestBuild(context.builds)
  return event.data.latest_build.updated_at !== latestBuildInTimeline.updated_at
}

const Language = {
  getTemplateWarning:
    "Error updating workspace: latest template could not be fetched.",
  getTemplateParametersWarning:
    "Error updating workspace: template parameters could not be fetched.",
  buildError: "Workspace action failed.",
}

type Permissions = Record<keyof ReturnType<typeof permissionsToCheck>, boolean>

export interface WorkspaceContext {
  // our server side events instance
  eventSource?: EventSource
  workspace?: TypesGen.Workspace
  template?: TypesGen.Template
  templateParameters?: TypesGen.TemplateVersionParameter[]
  build?: TypesGen.WorkspaceBuild
  getWorkspaceError?: Error | unknown
  // these are labeled as warnings because they don't make the page unusable
  refreshWorkspaceWarning?: Error | unknown
  getTemplateWarning: Error | unknown
  getTemplateParametersWarning: Error | unknown
  // Builds
  builds?: TypesGen.WorkspaceBuild[]
  getBuildsError?: Error | unknown
  // error creating a new WorkspaceBuild
  buildError?: Error | unknown
  cancellationMessage?: Types.Message
  cancellationError?: Error | unknown
  // permissions
  permissions?: Permissions
  checkPermissionsError?: Error | unknown
  // applications
  applicationsHost?: string
}

export type WorkspaceEvent =
  | { type: "GET_WORKSPACE"; workspaceName: string; username: string }
  | { type: "REFRESH_WORKSPACE"; data: TypesGen.ServerSentEvent["data"] }
  | { type: "START" }
  | { type: "STOP" }
  | { type: "ASK_DELETE" }
  | { type: "DELETE" }
  | { type: "CANCEL_DELETE" }
  | { type: "UPDATE" }
  | { type: "CANCEL" }
  | {
      type: "REFRESH_TIMELINE"
      checkRefresh?: boolean
      data?: TypesGen.ServerSentEvent["data"]
    }
  | { type: "EVENT_SOURCE_ERROR"; error: Error | unknown }
  | { type: "INCREASE_DEADLINE"; hours: number }
  | { type: "DECREASE_DEADLINE"; hours: number }

export const checks = {
  readWorkspace: "readWorkspace",
  updateWorkspace: "updateWorkspace",
} as const

const permissionsToCheck = (workspace: TypesGen.Workspace) => ({
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
})

export const workspaceMachine = createMachine(
  {
    id: "workspaceState",
    predictableActionArguments: true,
    tsTypes: {} as import("./workspaceXService.typegen").Typegen0,
    schema: {
      context: {} as WorkspaceContext,
      events: {} as WorkspaceEvent,
      services: {} as {
        getWorkspace: {
          data: TypesGen.Workspace
        }
        getTemplate: {
          data: TypesGen.Template
        }
        getTemplateParameters: {
          data: TypesGen.TemplateVersionParameter[]
        }
        startWorkspaceWithLatestTemplate: {
          data: TypesGen.WorkspaceBuild
        }
        startWorkspace: {
          data: TypesGen.WorkspaceBuild
        }
        stopWorkspace: {
          data: TypesGen.WorkspaceBuild
        }
        deleteWorkspace: {
          data: TypesGen.WorkspaceBuild
        }
        cancelWorkspace: {
          data: Types.Message
        }
        listening: {
          data: TypesGen.ServerSentEvent
        }
        getBuilds: {
          data: TypesGen.WorkspaceBuild[]
        }
        checkPermissions: {
          data: TypesGen.AuthorizationResponse
        }
        getApplicationsHost: {
          data: TypesGen.AppHostResponse
        }
      },
    },
    initial: "idle",
    on: {
      GET_WORKSPACE: {
        target: ".gettingWorkspace",
        internal: false,
      },
    },
    states: {
      idle: {
        tags: "loading",
      },
      gettingWorkspace: {
        entry: ["clearContext"],
        invoke: {
          src: "getWorkspace",
          id: "getWorkspace",
          onDone: [
            {
              actions: ["assignWorkspace", "clearGetWorkspaceError"],
              target: "gettingTemplate",
            },
          ],
          onError: [
            {
              actions: "assignGetWorkspaceError",
              target: "error",
            },
          ],
        },
        tags: "loading",
      },
      gettingTemplate: {
        invoke: {
          src: "getTemplate",
          id: "getTemplate",
          onDone: [
            {
              actions: ["assignTemplate", "clearGetTemplateWarning"],
              target: "gettingTemplateParameters",
            },
          ],
          onError: [
            {
              actions: [
                "assignGetTemplateWarning",
                "displayGetTemplateWarning",
              ],
              target: "error",
            },
          ],
        },
        tags: "loading",
      },
      gettingTemplateParameters: {
        invoke: {
          src: "getTemplateParameters",
          id: "getTemplateParameters",
          onDone: [
            {
              actions: [
                "assignTemplateParameters",
                "clearGetTemplateParametersWarning",
              ],
              target: "gettingPermissions",
            },
          ],
          onError: [
            {
              actions: [
                "assignGetTemplateParametersWarning",
                "displayGetTemplateParametersWarning",
              ],
              target: "error",
            },
          ],
        },
        tags: "loading",
      },
      gettingPermissions: {
        invoke: {
          src: "checkPermissions",
          id: "checkPermissions",
          onDone: [
            {
              actions: ["assignPermissions", "clearGetPermissionsError"],
              target: "ready",
            },
          ],
          onError: [
            {
              actions: "assignGetPermissionsError",
              target: "error",
            },
          ],
        },
        tags: "loading",
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
                    actions: [
                      "refreshWorkspace",
                      "clearRefreshWorkspaceWarning",
                    ],
                  },
                  EVENT_SOURCE_ERROR: {
                    target: "error",
                  },
                },
              },
              error: {
                entry: "assignRefreshWorkspaceWarning",
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
                  UPDATE: "updatingWorkspace",
                  CANCEL: "requestingCancel",
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
              updatingWorkspace: {
                tags: "updating",
                initial: "refreshingTemplate",
                states: {
                  refreshingTemplate: {
                    invoke: {
                      id: "refreshTemplate",
                      src: "getTemplate",
                      onDone: {
                        target: "startingWithLatestTemplate",
                        actions: ["assignTemplate"],
                      },
                      onError: {
                        target: "#workspaceState.ready.build.idle",
                        actions: ["assignGetTemplateWarning"],
                      },
                    },
                  },
                  startingWithLatestTemplate: {
                    invoke: {
                      id: "startWorkspaceWithLatestTemplate",
                      src: "startWorkspaceWithLatestTemplate",
                      onDone: {
                        target: "#workspaceState.ready.build.idle",
                        actions: ["assignBuild"],
                      },
                      onError: {
                        target: "#workspaceState.ready.build.idle",
                        actions: ["assignBuildError"],
                      },
                    },
                  },
                },
              },
              requestingStart: {
                entry: ["clearBuildError", "updateStatusToPending"],
                invoke: {
                  src: "startWorkspace",
                  id: "startWorkspace",
                  onDone: [
                    {
                      actions: ["assignBuild"],
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
                entry: ["clearBuildError", "updateStatusToPending"],
                invoke: {
                  src: "stopWorkspace",
                  id: "stopWorkspace",
                  onDone: [
                    {
                      actions: ["assignBuild"],
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
                entry: ["clearBuildError", "updateStatusToPending"],
                invoke: {
                  src: "deleteWorkspace",
                  id: "deleteWorkspace",
                  onDone: [
                    {
                      actions: ["assignBuild"],
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
                entry: [
                  "clearCancellationMessage",
                  "clearCancellationError",
                  "updateStatusToPending",
                ],
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
          applications: {
            initial: "gettingApplicationsHost",
            states: {
              gettingApplicationsHost: {
                invoke: {
                  src: "getApplicationsHost",
                  onDone: {
                    target: "success",
                    actions: ["assignApplicationsHost"],
                  },
                  onError: {
                    target: "error",
                    actions: ["displayApplicationsHostError"],
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
              src: workspaceScheduleBannerMachine,
              data: {
                workspace: (context: WorkspaceContext) => context.workspace,
              },
            },
          },
        },
      },
      error: {
        on: {
          GET_WORKSPACE: {
            target: "gettingWorkspace",
          },
        },
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
      assignWorkspace: assign({
        workspace: (_, event) => event.data,
      }),
      assignGetWorkspaceError: assign({
        getWorkspaceError: (_, event) => event.data,
      }),
      clearGetWorkspaceError: (context) =>
        assign({ ...context, getWorkspaceError: undefined }),
      assignTemplate: assign({
        template: (_, event) => event.data,
      }),
      assignTemplateParameters: assign({
        templateParameters: (_, event) => event.data,
      }),
      assignPermissions: assign({
        // Setting event.data as Permissions to be more stricted. So we know
        // what permissions we asked for.
        permissions: (_, event) => event.data as Permissions,
      }),
      assignGetPermissionsError: assign({
        checkPermissionsError: (_, event) => event.data,
      }),
      clearGetPermissionsError: assign({
        checkPermissionsError: (_) => undefined,
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
          displaySuccess(context.cancellationMessage.message)
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
      assignRefreshWorkspaceWarning: assign({
        refreshWorkspaceWarning: (_, event) => event,
      }),
      clearRefreshWorkspaceWarning: assign({
        refreshWorkspaceWarning: (_) => undefined,
      }),
      assignGetTemplateWarning: assign({
        getTemplateWarning: (_, event) => event.data,
      }),
      displayGetTemplateWarning: () => {
        displayError(Language.getTemplateWarning)
      },
      clearGetTemplateWarning: assign({
        getTemplateWarning: (_) => undefined,
      }),
      assignGetTemplateParametersWarning: assign({
        getTemplateParametersWarning: (_, event) => event.data,
      }),
      displayGetTemplateParametersWarning: () => {
        displayError(Language.getTemplateParametersWarning)
      },
      clearGetTemplateParametersWarning: assign({
        getTemplateParametersWarning: (_) => undefined,
      }),
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
      // Applications
      assignApplicationsHost: assign({
        applicationsHost: (_, { data }) => data.host,
      }),
      displayApplicationsHostError: (_, { data }) => {
        const message = getErrorMessage(
          data,
          "Error getting the applications host.",
        )
        displayError(message)
      },
      // Optimistically update. So when the user clicks on stop, we can show
      // the "pending" state right away without having to wait 0.5s ~ 2s to
      // display the visual feedback to the user.
      updateStatusToPending: assign({
        workspace: ({ workspace }) => {
          if (!workspace) {
            throw new Error("Workspace not defined")
          }

          return {
            ...workspace,
            latest_build: {
              ...workspace.latest_build,
              status: "pending" as TypesGen.WorkspaceStatus,
            },
          }
        },
      }),
    },
    guards: {
      moreBuildsAvailable,
    },
    services: {
      getWorkspace: async (_, event) => {
        return await API.getWorkspaceByOwnerAndName(
          event.username,
          event.workspaceName,
          {
            include_deleted: true,
          },
        )
      },
      getTemplate: async (context) => {
        if (context.workspace) {
          return await API.getTemplate(context.workspace.template_id)
        } else {
          throw Error("Cannot get template without workspace")
        }
      },
      getTemplateParameters: async (context) => {
        if (context.workspace) {
          return await API.getTemplateVersionRichParameters(
            context.workspace.latest_build.template_version_id,
          )
        } else {
          throw Error("Cannot get template parameters without workspace")
        }
      },
      startWorkspaceWithLatestTemplate: (context) => async (send) => {
        if (context.workspace && context.template) {
          const startWorkspacePromise = await API.startWorkspace(
            context.workspace.id,
            context.template.active_version_id,
          )
          send({ type: "REFRESH_TIMELINE" })
          return startWorkspacePromise
        } else {
          throw Error("Cannot start workspace without workspace id")
        }
      },
      startWorkspace: (context) => async (send) => {
        if (context.workspace) {
          const startWorkspacePromise = await API.startWorkspace(
            context.workspace.id,
            context.workspace.latest_build.template_version_id,
          )
          send({ type: "REFRESH_TIMELINE" })
          return startWorkspacePromise
        } else {
          throw Error("Cannot start workspace without workspace id")
        }
      },
      stopWorkspace: (context) => async (send) => {
        if (context.workspace) {
          const stopWorkspacePromise = await API.stopWorkspace(
            context.workspace.id,
          )
          send({ type: "REFRESH_TIMELINE" })
          return stopWorkspacePromise
        } else {
          throw Error("Cannot stop workspace without workspace id")
        }
      },
      deleteWorkspace: async (context) => {
        if (context.workspace) {
          const deleteWorkspacePromise = await API.deleteWorkspace(
            context.workspace.id,
          )
          send({ type: "REFRESH_TIMELINE" })
          return deleteWorkspacePromise
        } else {
          throw Error("Cannot delete workspace without workspace id")
        }
      },
      cancelWorkspace: (context) => async (send) => {
        if (context.workspace) {
          const cancelWorkspacePromise = await API.cancelWorkspaceBuild(
            context.workspace.latest_build.id,
          )
          send({ type: "REFRESH_TIMELINE" })
          return cancelWorkspacePromise
        } else {
          throw Error("Cannot cancel workspace without build id")
        }
      },
      listening: (context) => (send) => {
        if (!context.eventSource) {
          send({ type: "EVENT_SOURCE_ERROR", error: "error initializing sse" })
          return
        }

        context.eventSource.addEventListener("data", (event) => {
          // refresh our workspace with each SSE
          send({ type: "REFRESH_WORKSPACE", data: JSON.parse(event.data) })
          // refresh our timeline
          send({
            type: "REFRESH_TIMELINE",
            checkRefresh: true,
            data: JSON.parse(event.data),
          })
        })

        // handle any error events returned by our sse
        context.eventSource.addEventListener("error", (event) => {
          send({ type: "EVENT_SOURCE_ERROR", error: event })
        })

        // handle any sse implementation exceptions
        context.eventSource.onerror = () => {
          send({ type: "EVENT_SOURCE_ERROR", error: "sse error" })
        }
      },
      getBuilds: async (context) => {
        if (context.workspace) {
          // For now, we only retrieve the last month of builds to minimize
          // page bloat. We should add pagination in the future.
          return await API.getWorkspaceBuilds(
            context.workspace.id,
            dayjs().add(-30, "day").toDate(),
          )
        } else {
          throw Error("Cannot get builds without id")
        }
      },
      checkPermissions: async (context) => {
        if (context.workspace) {
          return await API.checkAuthorization({
            checks: permissionsToCheck(context.workspace),
          })
        } else {
          throw Error("Cannot check permissions workspace id")
        }
      },
      getApplicationsHost: async () => {
        return API.getApplicationsHost()
      },
    },
  },
)
