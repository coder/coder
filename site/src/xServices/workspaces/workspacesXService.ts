import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"
import { workspaceQueryToFilter } from "../../util/workspace"

interface WorkspaceContext {
  workspaces?: TypesGen.Workspace[]
  filter?: string
  getWorkspacesError?: Error | unknown
}

type WorkspaceEvent = { type: "GET_WORKSPACE"; workspaceId: string } | { type: "SET_FILTER"; query: string }

export const workspacesMachine = createMachine(
  {
    tsTypes: {} as import("./workspacesXService.typegen").Typegen0,
    schema: {
      context: {} as WorkspaceContext,
      events: {} as WorkspaceEvent,
      services: {} as {
        getWorkspaces: {
          data: TypesGen.Workspace[]
        }
      },
    },
    id: "workspaceState",
    initial: "ready",
    states: {
      ready: {
        on: {
          SET_FILTER: "extractingFilter",
        },
      },
      extractingFilter: {
        entry: "assignFilter",
        always: {
          target: "gettingWorkspaces",
        },
      },
      gettingWorkspaces: {
        entry: "clearGetWorkspacesError",
        invoke: {
          src: "getWorkspaces",
          id: "getWorkspaces",
          onDone: {
            target: "ready",
            actions: ["assignWorkspaces", "clearGetWorkspacesError"],
          },
          onError: {
            target: "ready",
            actions: ["assignGetWorkspacesError", "clearWorkspaces"],
          },
        },
        tags: "loading",
      },
    },
  },
  {
    actions: {
      assignWorkspaces: assign({
        workspaces: (_, event) => event.data,
      }),
      assignFilter: assign({
        filter: (_, event) => event.query,
      }),
      assignGetWorkspacesError: assign({
        getWorkspacesError: (_, event) => event.data,
      }),
      clearGetWorkspacesError: (context) => assign({ ...context, getWorkspacesError: undefined }),
      clearWorkspaces: (context) => assign({ ...context, workspaces: undefined }),
    },
    services: {
      getWorkspaces: (context) => API.getWorkspaces(workspaceQueryToFilter(context.filter)),
    },
  },
)
