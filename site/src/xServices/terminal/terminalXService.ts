import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"

// TypeScript doesn't have the randomUUID type on Crypto yet. See:
// https://github.com/denoland/deno/issues/12754#issuecomment-970386235
declare global {
  interface Crypto {
    randomUUID: () => string
  }
}

export interface TerminalContext {
  organizationsError?: Error | unknown
  organizations?: Types.Organization[]
  workspaceError?: Error | unknown
  workspace?: Types.Workspace
  workspaceAgent?: Types.WorkspaceAgent
  workspaceAgentError?: Error | unknown

  reconnection: string
  websocket?: WebSocket
}

export type TerminalEvent =
  | { type: "CONNECT" }
  | { type: "WRITE"; data: Types.ReconnectingPTYRequest }
  | { type: "READ"; data: string }

export const terminalMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QBcwCcC2BLAdgQwBsBlZPVAOhmWVygHk0o8csAvMrAex1gGIJuYcrgBunANZCqDJi3Y1u8JCAAOnWFgU5EoAB6IALAYAM5AwE5LANgAcAJgDsNqwGYrxqwBoQAT0QBWAEZTBysw8wc3W2NAg38AX3jvVExcQhIyKTBqWhlmNg5FXnQ0TjRyFQIyADMyjEpsvLlCnh1VdU0ubWV9BCNTC2t7J1d3L19EaPJIlzdzY39h20Tk9Gx8YlIKKhocKAB1MvFYFTwAYzB+QWEcMUkG5EO0Y9OLtrUNLTbe4ONzckClnMLjsBjcdg83j8CDc-jMszccRcCzsVgSSRAKXW6S2WRyeyeL3OlxKZQqVWQtUwD0JJ2J7w6Xx6iF+-0BlhBYKsEPG0P8BkC5BsCIMDgcBhsxkcdhWmLWaU2mQeuwORzpFwAgjAcMgrjghKIJHjaa8wFqwDqGZ8ut8WVZAnYhQ6LKKDFZrFDEHZkeR-AigsibGK7OYZRisQqMttsiqTcTzTrimhSuVKjU6jS1aaE8grZ1uLaEIF7Y6bM7zK73eZeYZjAMgUH7UHUeZZRGNlGhGduPqziq9QbbkbyN2cL3c8oPvnunovQ7HeYHe4bHFFjZ-J6i27yHXd-b5qN-OjVqkO7iRz2wH3aEmU+T09TR+O80zZwg7PPyIvUcYV0ebOum6ooK7h1oE4qBEEgLgW28pnkqT5XqgEC8PsABKACSAAqACiL42sy74uEY5AltybrulKDibvMX5AuY66-vYooOLBp44kqpJoLwADCdAAHL8ThPFYfhBaER+DHkHYjagi4K7ETYNE2Duu6-kxFj+BEiQYjgnAQHAbTthx0b4vQjD5PIXRKKAU6viAvQGHYm5WE55DybMcTCmEgQweGcEmXisZZvSk6MgRb4+TuoLVsCdigos1ETAgAY7mEti+X6aKWGx2KKqZwXPOqZrahOtnheJb4Og6Tqgg4gIOKiwoGJuLhHtMIqhC45j+E4uWRueiHXnsYkzg5LLrlY0zcv4riQQ6DjGC4QEgkKCJtX8s0uIEbj9fBFBDcho2Ft6bhCjNYrcqGqIbslgT2EKanVg48z2DYe2BeQEBYLAh2QMdhGxAY0keKi7oSrNELOclvUuO5wpRAxApBqx-nsflQhcQDb6nVNzh2L1oQhvFaKbgKU3dUCTiSo1LioyeeWdtj41Fkpd0OHRQJjEGgLOKjiRAA */
  createMachine(
    {
      tsTypes: {} as import("./terminalXService.typegen").Typegen0,
      schema: {
        context: {
          reconnection: crypto.randomUUID(),
        } as TerminalContext,
        events: {} as TerminalEvent,
        services: {} as {
          getOrganizations: {
            data: Types.Organization[]
          }
          getWorkspace: {
            data: Types.Workspace
          }
          getWorkspaceAgent: {
            data: Types.WorkspaceAgent
          }
          connect: {
            data: WebSocket
          }
        },
      },
      id: "terminalState",
      initial: "gettingOrganizations",
      states: {
        gettingOrganizations: {
          invoke: {
            src: "getOrganizations",
            id: "getOrganizations",
            onDone: [
              {
                actions: ["assignOrganizations", "clearOrganizationsError"],
                target: "gettingWorkspace",
              },
            ],
            onError: [
              {
                actions: "assignOrganizationsError",
                target: "error",
              },
            ],
          },
          tags: "loading",
        },
        gettingWorkspace: {
          invoke: {
            src: "getWorkspace",
            id: "getWorkspace",
            onDone: [
              {
                actions: ["assignWorkspace", "clearWorkspaceError"],
                target: "gettingWorkspaceAgent",
              },
            ],
            onError: [
              {
                actions: "assignWorkspaceError",
                target: "error",
              },
            ],
          },
        },
        gettingWorkspaceAgent: {
          invoke: {
            src: "getWorkspaceAgent",
            id: "getWorkspaceAgent",
            onDone: [
              {
                actions: ["assignWorkspaceAgent", "clearWorkspaceAgentError"],
                target: "connecting",
              },
            ],
            onError: [
              {
                actions: "assignWorkspaceAgentError",
                target: "error",
              },
            ],
          },
        },
        connecting: {
          invoke: {
            src: "connect",
            id: "connect",
            onDone: [
              {
                actions: ["assignWebsocket", "clearWebsocketError"],
                target: "connected",
              },
            ],
            onError: [
              {
                actions: "assignWebsocketError",
                target: "error",
              },
            ],
          },
        },
        connected: {
          on: {
            WRITE: {
              actions: "sendMessage",
            },
          },
        },
        disconnected: {},
        error: {
          on: {
            CONNECT: {
              target: "gettingOrganizations",
            },
          },
        },
      },
    },
    {
      services: {
        getOrganizations: API.getOrganizations,
        getWorkspace: (context: TerminalContext) => {
          return API.getWorkspace(context.organizations![0].id, "")
        },
        getWorkspaceAgent: async (context: TerminalContext) => {
          const resources = await API.getWorkspaceResources(context.workspace!.latest_build.id)
          for (let i = 0; i < resources.length; i++) {
            const resource = resources[i]
            if (resource.agents.length <= 0) {
              continue
            }
            return resource.agents[0]
          }
          throw new Error("no agent found with id")
        },
        connect: (context: TerminalContext) => (send) => {
          return new Promise<WebSocket>((resolve, reject) => {
            const socket = new WebSocket(`/api/v2/workspaceagents/${context.workspaceAgent!.id}/pty`)
            socket.addEventListener("open", () => {
              resolve(socket)
            })
            socket.addEventListener("error", (event) => {
              reject("socket errored")
            })
            socket.addEventListener("close", (event) => {
              reject(event.reason)
            })
            socket.addEventListener("message", (event) => {
              send({
                type: "READ",
                data: event.data,
              })
            })
          })
        },
      },
      actions: {
        assignOrganizations: assign({
          organizations: (_, event) => event.data,
        }),
        assignOrganizationsError: assign({
          organizationsError: (_, event) => event.data,
        }),
        clearOrganizationsError: assign((context: TerminalContext) => ({
          ...context,
          organizationsError: undefined,
        })),
        assignWorkspace: assign({
          workspace: (_, event) => event.data,
        }),
        assignWorkspaceError: assign({
          workspaceError: (_, event) => event.data,
        }),
        clearWorkspaceError: assign((context: TerminalContext) => ({
          ...context,
          workspaceError: undefined,
        })),
        assignWorkspaceAgent: assign({
          workspaceAgent: (_, event) => event.data,
        }),
        assignWorkspaceAgentError: assign({
          workspaceAgentError: (_, event) => event.data,
        }),
        clearWorkspaceAgentError: assign((context: TerminalContext) => ({
          ...context,
          workspaceAgentError: undefined,
        })),
        sendMessage: (context, event) => {
          context.websocket!.send(JSON.stringify(event.data))
        },
      },
    },
  )
