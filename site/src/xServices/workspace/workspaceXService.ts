import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"

interface WorkspaceContext {
  workspace?: Types.Workspace
  template?: Types.Template
  organization?: Types.Organization
  getWorkspaceError?: Error | unknown
  getTemplateError?: Error | unknown
  getOrganizationError?: Error | unknown
  // error enqueuing a ProvisionerJob to create a new WorkspaceBuild
  jobError?: Error | unknown
  // error creating a new WorkspaceBuild
  buildError?: Error | unknown
}

type WorkspaceEvent =
  | { type: "GET_WORKSPACE"; workspaceId: string }
  | { type: "START" }
  | { type: "STOP" }
  | { type: "RETRY" }
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
        invoke: {
          src: "getWorkspace",
          id: "getWorkspace",
          onDone: {
            target: "ready",
            actions: ["assignWorkspace", "clearGetWorkspaceError"],
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
                    actions: "clearJobError",
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
                  onDone: { target: "buildingStop", actions: "clearJobError" },
                  onError: {
                    target: "error",
                    actions: "assignJobError",
                  },
                },
                tags: ["buildLoading", "stopping"],
              },
              buildingStart: {
                invoke: {
                  id: "building",
                  src: "pollBuild",
                },
                initial: "refreshingWorkspace",
                states: {
                  refreshingWorkspace: {
                    invoke: {
                      id: "refreshWorkspace",
                      src: "refreshWorkspace",
                      onDone: [
                        {
                          cond: "jobSucceeded",
                          target: "#workspaceState.ready.build.started",
                          actions: ["clearBuildError", "assignWorkspace"],
                        },
                        {
                          cond: "jobPendingOrRunning",
                          target: "waiting",
                          actions: "assignWorkspace"
                        },
                        {
                          // if job is canceling, cancelled, or failed, the user needs to retry
                          target: "#workspaceState.ready.build.error",
                          actions: ["assignBuildError", "assignWorkspace"],
                        },
                      ],
                      onError: "waiting",
                    },
                  },
                  waiting: {
                    on: {
                      REFRESH_WORKSPACE: "refreshingWorkspace",
                    },
                  },
                },
                tags: ["buildLoading", "starting"],
              },
              buildingStop: {
                invoke: {
                  id: "building",
                  src: "pollBuild",
                },
                initial: "refreshingWorkspace",
                states: {
                  refreshingWorkspace: {
                    invoke: {
                      id: "refreshWorkspace",
                      src: "refreshWorkspace",
                      onDone: [
                        {
                          cond: "jobSucceeded",
                          target: "#workspaceState.ready.build.stopped",
                          actions: "clearBuildError",
                        },
                        {
                          cond: "jobPendingOrRunning",
                          target: "waiting",
                        },
                        {
                          // if job is canceling, cancelled, or failed, the user needs to retry
                          target: "#workspaceState.ready.build.error",
                          actions: "assignBuildError",
                        },
                      ],
                      onError: "waiting",
                    },
                  },
                  waiting: {
                    on: {
                      REFRESH_WORKSPACE: "refreshingWorkspace",
                    },
                  },
                },
                tags: ["buildLoading", "stopping"],
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
      pollBuild: async (context) => (send) => {
        if (context.workspace) {
          const workspaceId = context.workspace.id
          const intervalId = setInterval(() => send({ type: "GET_WORKSPACE", workspaceId }), 1000)
          return () => clearInterval(intervalId)
        } else {
          throw Error("Cannot fetch workspace without id")
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
