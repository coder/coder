import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"

interface WorkspaceContext {
  workspaces?: TypesGen.Workspace[]
  getWorkspacesError?: Error | unknown
}

type WorkspaceEvent = { type: "GET_WORKSPACE"; workspaceId: string }

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
    initial: "gettingWorkspaces",
    states: {
      gettingWorkspaces: {
        entry: "clearGetWorkspacesError",
        invoke: {
          src: "getWorkspaces",
          id: "getWorkspaces",
          onDone: {
            target: "done",
            actions: ["assignWorkspaces", "clearGetWorkspacesError"],
          },
          onError: {
            target: "error",
            actions: "assignGetWorkspacesError",
          },
        },
        tags: "loading",
      },
      done: {},
      error: {},
    },
  },
  {
    actions: {
      assignWorkspaces: assign({
        workspaces: (_, event) => event.data,
      }),
      assignGetWorkspacesError: assign({
        getWorkspacesError: (_, event) => event.data,
      }),
      clearGetWorkspacesError: (context) => assign({ ...context, getWorkspacesError: undefined }),
    },
    services: {
      getWorkspaces: () => API.getWorkspaces(),
    },
  },
)
