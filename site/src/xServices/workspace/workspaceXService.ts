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
}

type WorkspaceEvent = { type: "GET_WORKSPACE"; workspaceId: string } | { type: "START" } | {type: "STOP"} | { type: "RETRY" }

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
        pollBuild: {
          data: Types.Workspace
        }
      },
    },
    id: "workspaceState",
    initial: "idle",
    states: {
      idle: {
        on: {
          GET_WORKSPACE: "gettingWorkspace",
        },
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
              ready: {}
            },
          },
          build: {
            initial: "idle",
            states: {
              idle: {
                on: {
                  START: "requestingStart",
                  STOP: "requestingStop"
                }
              },
              requestingStart: {
                invoke: {
                  id: "startWorkspace",
                  src: "startWorkspace",
                  onDone: "building",
                  onError: {
                    target: "error",
                    actions: ["assignFailedTransition", "assignEnqueueError"]
                  }
                }
              },
              requestingStop: {
                invoke: {
                  id: "stopWorkspace",
                  src: "stopWorkspace",
                  onDone: "building",
                  onError: {
                    target: "error",
                    actions: ["assignFailedTransition", "assignEnqueueError"]
                  }
                }
              },
              building: {
                invoke: {
                  id: "building",
                  src: "pollBuild",
                  onDone: "idle",
                  onError: {
                    target: "error",
                    actions: ["assignFailedTransition", "assignBuildError"]
                  }
                }
              },
              error: {
                on: {
                  RETRY: [
                    {
                      cond: "failedToStart",
                      target: "requestingStart"
                    },
                    {
                      target: "requestingStop"
                    }
                  ]
                }
              }
            }
          }
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
      }
    },
  },
)
