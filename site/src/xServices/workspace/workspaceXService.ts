import { assign, createMachine, send } from "xstate"
import { pure } from "xstate/lib/actions"
import * as API from "../../api/api"
import * as Types from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"

const latestBuild = (builds: TypesGen.WorkspaceBuild[]) => {
  // Cloning builds to not change the origin object with the sort()
  return [...builds].sort((a, b) => {
    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
  })[0]
}

const Language = {
  refreshTemplateError: "Error updating workspace: latest template could not be fetched.",
  buildError: "Workspace action failed.",
}

type Permissions = Record<keyof ReturnType<typeof permissionsToCheck>, boolean>

export interface WorkspaceContext {
  workspace?: TypesGen.Workspace
  template?: TypesGen.Template
  build?: TypesGen.WorkspaceBuild
  resources?: TypesGen.WorkspaceResource[]
  getWorkspaceError?: Error | unknown
  // error creating a new WorkspaceBuild
  buildError?: Error | unknown
  // these are separate from getX errors because they don't make the page unusable
  refreshWorkspaceError: Error | unknown
  refreshTemplateError: Error | unknown
  getResourcesError: Error | unknown
  // Builds
  builds?: TypesGen.WorkspaceBuild[]
  getBuildsError?: Error | unknown
  loadMoreBuildsError?: Error | unknown
  cancellationMessage?: Types.Message
  cancellationError?: Error | unknown
  // permissions
  permissions?: Permissions
  checkPermissionsError?: Error | unknown
  userId?: string
}

export type WorkspaceEvent =
  | { type: "GET_WORKSPACE"; workspaceName: string; username: string }
  | { type: "START" }
  | { type: "STOP" }
  | { type: "ASK_DELETE" }
  | { type: "DELETE" }
  | { type: "CANCEL_DELETE" }
  | { type: "UPDATE" }
  | { type: "CANCEL" }
  | { type: "LOAD_MORE_BUILDS" }
  | { type: "REFRESH_TIMELINE" }

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
        refreshWorkspace: {
          data: TypesGen.Workspace | undefined
        }
        getResources: {
          data: TypesGen.WorkspaceResource[]
        }
        getBuilds: {
          data: TypesGen.WorkspaceBuild[]
        }
        loadMoreBuilds: {
          data: TypesGen.WorkspaceBuild[]
        }
        checkPermissions: {
          data: TypesGen.UserAuthorizationResponse
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
        entry: ["clearGetWorkspaceError", "clearContext"],
        invoke: {
          src: "getWorkspace",
          id: "getWorkspace",
          onDone: [
            {
              actions: "assignWorkspace",
              target: "refreshingTemplate",
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
      refreshingTemplate: {
        entry: "clearRefreshTemplateError",
        invoke: {
          src: "getTemplate",
          id: "refreshTemplate",
          onDone: [
            {
              actions: "assignTemplate",
              target: "gettingPermissions",
            },
          ],
          onError: [
            {
              actions: ["assignRefreshTemplateError", "displayRefreshTemplateError"],
              target: "error",
            },
          ],
        },
        tags: "loading",
      },
      gettingPermissions: {
        entry: "clearGetPermissionsError",
        invoke: {
          src: "checkPermissions",
          id: "checkPermissions",
          onDone: [
            {
              actions: "assignPermissions",
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
      },
      ready: {
        type: "parallel",
        states: {
          pollingWorkspace: {
            initial: "refreshingWorkspace",
            states: {
              refreshingWorkspace: {
                entry: "clearRefreshWorkspaceError",
                invoke: {
                  src: "refreshWorkspace",
                  id: "refreshWorkspace",
                  onDone: [
                    {
                      actions: ["refreshTimeline", "assignWorkspace"],
                      target: "waiting",
                    },
                  ],
                  onError: [
                    {
                      actions: "assignRefreshWorkspaceError",
                      target: "waiting",
                    },
                  ],
                },
              },
              waiting: {
                after: {
                  "1000": {
                    target: "refreshingWorkspace",
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
                  UPDATE: "requestingStartWithLatestTemplate",
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
              requestingStartWithLatestTemplate: {
                entry: "clearBuildError",
                invoke: {
                  id: "startWorkspaceWithLatestTemplate",
                  src: "startWorkspaceWithLatestTemplate",
                  onDone: {
                    target: "idle",
                    actions: ["assignBuild", "refreshTimeline"],
                  },
                  onError: {
                    target: "idle",
                    actions: ["assignBuildError"],
                  },
                },
              },
              requestingStart: {
                entry: "clearBuildError",
                invoke: {
                  src: "startWorkspace",
                  id: "startWorkspace",
                  onDone: [
                    {
                      actions: ["assignBuild", "refreshTimeline"],
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
                entry: "clearBuildError",
                invoke: {
                  src: "stopWorkspace",
                  id: "stopWorkspace",
                  onDone: [
                    {
                      actions: ["assignBuild", "refreshTimeline"],
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
                entry: "clearBuildError",
                invoke: {
                  src: "deleteWorkspace",
                  id: "deleteWorkspace",
                  onDone: [
                    {
                      actions: ["assignBuild", "refreshTimeline"],
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
                        "refreshTimeline",
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
              refreshingTemplate: {
                entry: "clearRefreshTemplateError",
                invoke: {
                  src: "getTemplate",
                  id: "refreshTemplate",
                  onDone: [
                    {
                      actions: "assignTemplate",
                      target: "requestingStart",
                    },
                  ],
                  onError: [
                    {
                      actions: ["assignRefreshTemplateError", "displayRefreshTemplateError"],
                      target: "idle",
                    },
                  ],
                },
              },
            },
          },
          pollingResources: {
            initial: "gettingResources",
            states: {
              gettingResources: {
                entry: "clearGetResourcesError",
                invoke: {
                  src: "getResources",
                  id: "getResources",
                  onDone: [
                    {
                      actions: "assignResources",
                      target: "waiting",
                    },
                  ],
                  onError: [
                    {
                      actions: "assignGetResourcesError",
                      target: "waiting",
                    },
                  ],
                },
              },
              waiting: {
                after: {
                  "5000": {
                    target: "gettingResources",
                  },
                },
              },
            },
          },
          timeline: {
            initial: "gettingBuilds",
            states: {
              idle: {},
              gettingBuilds: {
                entry: "clearGetBuildsError",
                invoke: {
                  src: "getBuilds",
                  onDone: [
                    {
                      actions: "assignBuilds",
                      target: "loadedBuilds",
                    },
                  ],
                  onError: [
                    {
                      actions: "assignGetBuildsError",
                      target: "idle",
                    },
                  ],
                },
              },
              loadedBuilds: {
                initial: "idle",
                states: {
                  idle: {
                    on: {
                      LOAD_MORE_BUILDS: {
                        cond: "hasMoreBuilds",
                        target: "loadingMoreBuilds",
                      },
                      REFRESH_TIMELINE: {
                        target: "#workspaceState.ready.timeline.gettingBuilds",
                      },
                    },
                  },
                  loadingMoreBuilds: {
                    entry: "clearLoadMoreBuildsError",
                    invoke: {
                      src: "loadMoreBuilds",
                      onDone: [
                        {
                          actions: "assignNewBuilds",
                          target: "idle",
                        },
                      ],
                      onError: [
                        {
                          actions: "assignLoadMoreBuildsError",
                          target: "idle",
                        },
                      ],
                    },
                  },
                },
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
        }),
      assignWorkspace: assign({
        workspace: (_, event) => event.data,
      }),
      assignGetWorkspaceError: assign({
        getWorkspaceError: (_, event) => event.data,
      }),
      clearGetWorkspaceError: (context) => assign({ ...context, getWorkspaceError: undefined }),
      assignTemplate: assign({
        template: (_, event) => event.data,
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
      assignRefreshWorkspaceError: assign({
        refreshWorkspaceError: (_, event) => event.data,
      }),
      clearRefreshWorkspaceError: assign({
        refreshWorkspaceError: (_) => undefined,
      }),
      assignRefreshTemplateError: assign({
        refreshTemplateError: (_, event) => event.data,
      }),
      displayRefreshTemplateError: () => {
        displayError(Language.refreshTemplateError)
      },
      clearRefreshTemplateError: assign({
        refreshTemplateError: (_) => undefined,
      }),
      // Resources
      assignResources: assign({
        resources: (_, event) => event.data,
      }),
      assignGetResourcesError: assign({
        getResourcesError: (_, event) => event.data,
      }),
      clearGetResourcesError: assign({
        getResourcesError: (_) => undefined,
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
      assignNewBuilds: assign({
        builds: (context, event) => {
          const oldBuilds = context.builds

          if (!oldBuilds) {
            // This state is theoretically impossible, but helps TS
            throw new Error("workspaceXService: failed to load workspace builds")
          }

          return [...oldBuilds, ...event.data]
        },
      }),
      assignLoadMoreBuildsError: assign({
        loadMoreBuildsError: (_, event) => event.data,
      }),
      clearLoadMoreBuildsError: assign({
        loadMoreBuildsError: (_) => undefined,
      }),
      refreshTimeline: pure((context, event) => {
        // No need to refresh the timeline if it is not loaded
        if (!context.builds) {
          return
        }
        // When it is a refresh workspace event, we want to check if the latest
        // build was updated to not over fetch the builds
        if (event.type === "done.invoke.refreshWorkspace") {
          const latestBuildInTimeline = latestBuild(context.builds)
          const isUpdated = event.data?.latest_build.updated_at !== latestBuildInTimeline.updated_at
          if (isUpdated) {
            return send({ type: "REFRESH_TIMELINE" })
          }
        } else {
          return send({ type: "REFRESH_TIMELINE" })
        }
      }),
    },
    guards: {
      hasMoreBuilds: (_) => false,
    },
    services: {
      getWorkspace: async (_, event) => {
        return await API.getWorkspaceByOwnerAndName(event.username, event.workspaceName, {
          include_deleted: true,
        })
      },
      getTemplate: async (context) => {
        if (context.workspace) {
          return await API.getTemplate(context.workspace.template_id)
        } else {
          throw Error("Cannot get template without workspace")
        }
      },
      startWorkspaceWithLatestTemplate: async (context) => {
        if (context.workspace && context.template) {
          return await API.startWorkspace(context.workspace.id, context.template.active_version_id)
        } else {
          throw Error("Cannot start workspace without workspace id")
        }
      },
      startWorkspace: async (context) => {
        if (context.workspace) {
          return await API.startWorkspace(
            context.workspace.id,
            context.workspace.latest_build.template_version_id,
          )
        } else {
          throw Error("Cannot start workspace without workspace id")
        }
      },
      stopWorkspace: async (context) => {
        if (context.workspace) {
          return await API.stopWorkspace(context.workspace.id)
        } else {
          throw Error("Cannot stop workspace without workspace id")
        }
      },
      deleteWorkspace: async (context) => {
        if (context.workspace) {
          return await API.deleteWorkspace(context.workspace.id)
        } else {
          throw Error("Cannot delete workspace without workspace id")
        }
      },
      cancelWorkspace: async (context) => {
        if (context.workspace) {
          return await API.cancelWorkspaceBuild(context.workspace.latest_build.id)
        } else {
          throw Error("Cannot cancel workspace without build id")
        }
      },
      refreshWorkspace: async (context) => {
        if (context.workspace) {
          return await API.getWorkspaceByOwnerAndName(
            context.workspace.owner_name,
            context.workspace.name,
            {
              include_deleted: true,
            },
          )
        } else {
          throw Error("Cannot refresh workspace without id")
        }
      },
      getResources: async (context) => {
        // If the job hasn't completed, fetching resources will result
        // in an unfriendly error for the user.
        if (!context.workspace?.latest_build.job.completed_at) {
          return []
        }
        const resources = await API.getWorkspaceResources(context.workspace.latest_build.id)
        return resources
      },
      getBuilds: async (context) => {
        if (context.workspace) {
          return await API.getWorkspaceBuilds(context.workspace.id)
        } else {
          throw Error("Cannot get builds without id")
        }
      },
      loadMoreBuilds: async (context) => {
        if (context.workspace) {
          return await API.getWorkspaceBuilds(context.workspace.id)
        } else {
          throw Error("Cannot load more builds without id")
        }
      },
      checkPermissions: async (context) => {
        if (context.workspace && context.userId) {
          return await API.checkUserPermissions(context.userId, {
            checks: permissionsToCheck(context.workspace),
          })
        } else {
          throw Error("Cannot check permissions without both workspace and user id")
        }
      },
    },
  },
)
