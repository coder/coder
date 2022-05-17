import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"
import { displayError } from "../../components/GlobalSnackbar/utils"

const Language = {
  refreshTemplateError: "Error updating workspace: latest template could not be fetched.",
  buildError: "Workspace action failed.",
}

export interface WorkspaceContext {
  workspace?: TypesGen.Workspace
  template?: TypesGen.Template
  organization?: TypesGen.Organization
  build?: TypesGen.WorkspaceBuild
  getWorkspaceError?: Error | unknown
  getTemplateError?: Error | unknown
  getOrganizationError?: Error | unknown
  // error creating a new WorkspaceBuild
  buildError?: Error | unknown
  // these are separate from getX errors because they don't make the page unusable
  refreshWorkspaceError: Error | unknown
  refreshTemplateError: Error | unknown
}

export type WorkspaceEvent =
  | { type: "GET_WORKSPACE"; workspaceId: string }
  | { type: "START" }
  | { type: "STOP" }
  | { type: "RETRY" }
  | { type: "UPDATE" }

export const workspaceMachine = createMachine(
  {
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
        getOrganization: {
          data: TypesGen.Organization
        }
        startWorkspace: {
          data: TypesGen.WorkspaceBuild
        }
        stopWorkspace: {
          data: TypesGen.WorkspaceBuild
        }
        refreshWorkspace: {
          data: TypesGen.Workspace | undefined
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
          // We poll the workspace consistently to know if it becomes outdated and to update build status
          pollingWorkspace: {
            initial: "refreshingWorkspace",
            states: {
              refreshingWorkspace: {
                entry: "clearRefreshWorkspaceError",
                invoke: {
                  id: "refreshWorkspace",
                  src: "refreshWorkspace",
                  onDone: { target: "waiting", actions: "assignWorkspace" },
                  onError: { target: "waiting", actions: "assignRefreshWorkspaceError" },
                },
              },
              waiting: {
                after: {
                  1000: "refreshingWorkspace",
                },
              },
            },
          },
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
            initial: "idle",
            states: {
              idle: {
                on: {
                  START: "requestingStart",
                  STOP: "requestingStop",
                  RETRY: [{ cond: "triedToStart", target: "requestingStart" }, { target: "requestingStop" }],
                  UPDATE: "refreshingTemplate",
                },
              },
              requestingStart: {
                entry: "clearBuildError",
                invoke: {
                  id: "startWorkspace",
                  src: "startWorkspace",
                  onDone: {
                    target: "idle",
                    actions: "assignBuild",
                  },
                  onError: {
                    target: "idle",
                    actions: ["assignBuildError", "displayBuildError"],
                  },
                },
              },
              requestingStop: {
                entry: "clearBuildError",
                invoke: {
                  id: "stopWorkspace",
                  src: "stopWorkspace",
                  onDone: {
                    target: "idle",
                    actions: "assignBuild",
                  },
                  onError: {
                    target: "idle",
                    actions: ["assignBuildError", "displayBuildError"],
                  },
                },
              },
              refreshingTemplate: {
                entry: "clearRefreshTemplateError",
                invoke: {
                  id: "refreshTemplate",
                  src: "getTemplate",
                  onDone: {
                    target: "requestingStart",
                    actions: "assignTemplate",
                  },
                  onError: {
                    target: "idle",
                    actions: ["assignRefreshTemplateError", "displayRefreshTemplateError"],
                  },
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
      assignBuildError: (_, event) =>
        assign({
          buildError: event.data,
        }),
      displayBuildError: () => {
        displayError(Language.buildError)
      },
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
      displayRefreshTemplateError: () => {
        displayError(Language.refreshTemplateError)
      },
      clearRefreshTemplateError: (_) =>
        assign({
          refreshTemplateError: undefined,
        }),
    },
    guards: {
      triedToStart: (context) => context.workspace?.latest_build.transition === "start",
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
          return await API.startWorkspace(context.workspace.id, context.template?.active_version_id)
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
