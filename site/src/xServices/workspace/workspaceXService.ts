import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"

interface WorkspaceContext {
  workspace?: Types.Workspace
  template?: Types.Template
  organization?: Types.Organization
  build?: TypesGen.WorkspaceBuild
  getWorkspaceError?: Error | unknown
  getTemplateError?: Error | unknown
  getOrganizationError?: Error | unknown
  // error enqueuing a ProvisionerJob to create a new WorkspaceBuild
  jobError?: Error | unknown
  // error creating a new WorkspaceBuild
  buildError?: Error | unknown
  // these are separate from get X errors because they don't make the page unusable
  refreshWorkspaceError: Error | unknown
  refreshTemplateError: Error | unknown
}

type WorkspaceEvent =
  | { type: "GET_WORKSPACE"; workspaceId: string }
  | { type: "START" }
  | { type: "STOP" }
  | { type: "RETRY" }
  | { type: "UPDATE" }
  | { type: "REFRESH_WORKSPACE" }

export const workspaceMachine = createMachine(
  {
    tsTypes: {} as import("./workspaceXService.typegen").Typegen0,
    schema: {
      context: {} as WorkspaceContext,
      events: {} as WorkspaceEvent,
      services: {} as {
        getWorkspace: {
          data: Types.Workspace
        }
        getTemplate: {
          data: Types.Template
        }
        getOrganization: {
          data: Types.Organization
        }
        startWorkspace: {
          data: TypesGen.WorkspaceBuild
        }
        stopWorkspace: {
          data: TypesGen.WorkspaceBuild
        }
        refreshWorkspace: {
          data: Types.Workspace | undefined
        }
      },
    },
    id: "workspaceState",
    initial: "idle",
    on: {
      GET_WORKSPACE: "gettingWorkspace",
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
          onDone: {
            target: "ready",
            actions: ["assignWorkspace"],
          },
          onError: {
            target: "error",
            actions: "assignGetWorkspaceError",
          },
        },
        tags: "loading",
      },
      ready: {
        type: "parallel",
        states: {
          breadcrumb: {
            initial: "gettingTemplate",
            states: {
              gettingTemplate: {
                invoke: {
                  src: "getTemplate",
                  id: "getTemplate",
                  onDone: {
                    target: "gettingOrganization",
                    actions: ["assignTemplate", "clearGetTemplateError"],
                  },
                  onError: {
                    target: "error",
                    actions: "assignGetTemplateError",
                  },
                },
                tags: "loading",
              },
              gettingOrganization: {
                invoke: {
                  src: "getOrganization",
                  id: "getOrganization",
                  onDone: {
                    target: "ready",
                    actions: ["assignOrganization", "clearGetOrganizationError"],
                  },
                  onError: {
                    target: "error",
                    actions: "assignGetOrganizationError",
                  },
                },
                tags: "loading",
              },
              error: {},
              ready: {},
            },
          },
          build: {
            initial: "dispatch",
            on: {
              UPDATE: "#workspaceState.ready.build.refreshingTemplate",
            },
            states: {
              dispatch: {
                always: [
                  {
                    cond: "workspaceIsStarted",
                    target: "started",
                  },
                  {
                    cond: "workspaceIsStopped",
                    target: "stopped",
                  },
                  {
                    cond: "workspaceIsStarting",
                    target: "buildingStart",
                  },
                  {
                    cond: "workspaceIsStopping",
                    target: "buildingStop",
                  },
                  { target: "error" },
                ],
              },
              started: {
                on: {
                  STOP: "requestingStop",
                },
                tags: "buildReady",
              },
              stopped: {
                on: {
                  START: "requestingStart",
                },
                tags: "buildReady",
              },
              requestingStart: {
                invoke: {
                  id: "startWorkspace",
                  src: "startWorkspace",
                  onDone: {
                    target: "buildingStart",
                    actions: ["assignBuild", "clearJobError"],
                  },
                  onError: {
                    target: "error",
                    actions: "assignJobError",
                  },
                },
                tags: ["buildLoading", "starting"],
              },
              requestingStop: {
                invoke: {
                  id: "stopWorkspace",
                  src: "stopWorkspace",
                  onDone: { target: "buildingStop", actions: ["assignBuild", "clearJobError"] },
                  onError: {
                    target: "error",
                    actions: "assignJobError",
                  },
                },
                tags: ["buildLoading", "stopping"],
              },
              buildingStart: {
                initial: "refreshingWorkspace",
                states: {
                  refreshingWorkspace: {
                    entry: "clearRefreshWorkspaceError",
                    invoke: {
                      id: "refreshWorkspace",
                      src: "refreshWorkspace",
                      onDone: [
                        {
                          cond: "jobSucceeded",
                          target: "#workspaceState.ready.build.started",
                          actions: ["clearBuildError", "assignWorkspace"],
                        },
                        { cond: "jobPendingOrRunning", target: "waiting", actions: "assignWorkspace" },
                        {
                          target: "#workspaceState.ready.build.error",
                          actions: ["assignWorkspace", "assignBuildError"],
                        },
                      ],
                      onError: { target: "waiting", actions: "assignRefreshWorkspaceError" },
                    },
                  },
                  waiting: {
                    after: {
                      1000: "refreshingWorkspace",
                    },
                  },
                },
                tags: ["buildLoading", "starting"],
              },
              buildingStop: {
                initial: "refreshingWorkspace",
                states: {
                  refreshingWorkspace: {
                    entry: "clearRefreshWorkspaceError",
                    invoke: {
                      id: "refreshWorkspace",
                      src: "refreshWorkspace",
                      onDone: [
                        {
                          cond: "jobSucceeded",
                          target: "#workspaceState.ready.build.stopped",
                          actions: ["clearBuildError", "assignWorkspace"],
                        },
                        { cond: "jobPendingOrRunning", target: "waiting", actions: "assignWorkspace" },
                        {
                          target: "#workspaceState.ready.build.error",
                          actions: ["assignWorkspace", "assignBuildError"],
                        },
                      ],
                      onError: { target: "waiting", actions: "assignRefreshWorkspaceError" },
                    },
                  },
                  waiting: {
                    after: {
                      1000: "refreshingWorkspace",
                    },
                  },
                },
                tags: ["buildLoading", "stopping"],
              },
              refreshingTemplate: {
                entry: "clearRefreshTemplateError",
                invoke: {
                  id: "refreshTemplate",
                  src: "getTemplate",
                  onDone: { target: "#workspaceState.ready.build.requestingStart", actions: "assignTemplate" },
                  onError: { target: "error", actions: "assignRefreshTemplateError" },
                },
              },
              error: {
                on: {
                  RETRY: [
                    {
                      cond: "triedToStart",
                      target: "requestingStart",
                    },
                    {
                      // this could also be post-delete
                      target: "requestingStop",
                    },
                  ],
                },
              },
            },
          },
        },
      },
      error: {
        on: {
          GET_WORKSPACE: "gettingWorkspace",
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
          organization: undefined,
          build: undefined,
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
      assignGetTemplateError: assign({
        getTemplateError: (_, event) => event.data,
      }),
      clearGetTemplateError: (context) => assign({ ...context, getTemplateError: undefined }),
      assignOrganization: assign({
        organization: (_, event) => event.data,
      }),
      assignGetOrganizationError: assign({
        getOrganizationError: (_, event) => event.data,
      }),
      clearGetOrganizationError: (context) => assign({ ...context, getOrganizationError: undefined }),
      assignBuild: (_, event) =>
        assign({
          build: event.data,
        }),
      assignJobError: (_, event) =>
        assign({
          jobError: event.data,
        }),
      clearJobError: (_) =>
        assign({
          jobError: undefined,
        }),
      assignBuildError: (_, event) =>
        assign({
          buildError: event.data,
        }),
      clearBuildError: (_) =>
        assign({
          buildError: undefined,
        }),
      assignRefreshWorkspaceError: (_, event) =>
        assign({
          refreshWorkspaceError: event.data,
        }),
      clearRefreshWorkspaceError: (_) =>
        assign({
          refreshWorkspaceError: undefined,
        }),
      assignRefreshTemplateError: (_, event) =>
        assign({
          refreshTemplateError: event.data,
        }),
      clearRefreshTemplateError: (_) =>
        assign({
          refreshTemplateError: undefined,
        }),
    },
    guards: {
      workspaceIsStarted: (context) =>
        context.workspace?.latest_build.transition === "start" &&
        context.workspace.latest_build.job.status === "succeeded",
      workspaceIsStopped: (context) =>
        context.workspace?.latest_build.transition === "stop" &&
        context.workspace.latest_build.job.status === "succeeded",
      workspaceIsStarting: (context) =>
        context.workspace?.latest_build.transition === "start" &&
        ["pending", "running"].includes(context.workspace.latest_build.job.status),
      workspaceIsStopping: (context) =>
        context.workspace?.latest_build.transition === "stop" &&
        ["pending", "running"].includes(context.workspace.latest_build.job.status),
      triedToStart: (context) => context.workspace?.latest_build.transition === "start",
      jobSucceeded: (context) => context.workspace?.latest_build.job.status === "succeeded",
      jobPendingOrRunning: (context) => {
        const status = context.workspace?.latest_build.job.status
        return status === "pending" || status === "running"
      },
    },
    services: {
      getWorkspace: async (_, event) => {
        return await API.getWorkspace(event.workspaceId)
      },
      getTemplate: async (context) => {
        if (context.workspace) {
          return await API.getTemplate(context.workspace.template_id)
        } else {
          throw Error("Cannot get template without workspace")
        }
      },
      getOrganization: async (context) => {
        if (context.template) {
          return await API.getOrganization(context.template.organization_id)
        } else {
          throw Error("Cannot get organization without template")
        }
      },
      startWorkspace: async (context) => {
        if (context.workspace) {
          return await API.startWorkspace(context.workspace.id)
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
      refreshWorkspace: async (context) => {
        if (context.workspace) {
          return await API.getWorkspace(context.workspace.id)
        } else {
          throw Error("Cannot refresh workspace without id")
        }
      },
    },
  },
)
