import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"

interface WorkspaceContext {
  workspace?: Types.Workspace
  template?: Types.Template
  organization?: Types.Organization
  getWorkspaceError?: Error | unknown
  getTemplateError?: Error | unknown
  getOrganizationError?: Error | unknown
}

type WorkspaceEvent = { type: "GET_WORKSPACE"; workspaceId: string }

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
            target: "gettingTemplate",
            actions: ["assignWorkspace", "clearGetWorkspaceError"],
          },
          onError: {
            target: "error",
            actions: "assignGetWorkspaceError",
          },
        },
        tags: "loading",
      },
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
            target: "idle",
            actions: ["assignOrganization", "clearGetOrganizationError"],
          },
          onError: {
            target: "error",
            actions: "assignGetOrganizationError",
          },
        },
        tags: "loading",
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
    },
  },
)
