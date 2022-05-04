import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"
import * as TypesGen from "../../api/typesGenerated"

    `/api/v2/workspaces/${workspaceParam}`
     `/api/v2/templates/${unsafeSWRArgument(workspace).template_id}`
     `/api/v2/organizations/${unsafeSWRArgument(template).organization_id}`

interface WorkspaceContext {
  workspace?: Types.Workspace,
  template?: Types.Template,
  organization?: Types.Organization
}

type WorkspaceEvent = { type: "GET_WORKSPACE", workspaceName: string, organizationID: string }

export const workspaceMachine = createMachine({
  tsTypes: {} as import("./workspaceXService.typegen").Typegen0,
  schema: {
    context: {} as WorkspaceContext,
    events: {} as WorkspaceEvent,
    services: {} as {
      getWorkspace: {
        data: Types.Workspace,
      },
      getTemplate: {
        data: Types.Template,
      }
      getOrganization: {
        data: Types.Organization,
      }
    }
  },
  id: "workspaceState",
  initial: "idle",
  states: {
    idle: {
      on: {
        GET_WORKSPACE: "gettingWorkspace"
      }
    },
    gettingWorkspace: {
      invoke: {
        src: "getWorkspace",
        id: 'getWorkspace',
        onDone: {},
        onError: {}
      }
    },
    gettingTemplate: {},
    gettingOrganization: {},
    error: {}
  }
}, {
  actions: {},
  services: {
    getWorkspace: async (_, event) => {
      return await API.getWorkspace(
        event.organizationID,
        "me",
        event.workspaceName)
    }
  }
})
