import { assign, createMachine } from "xstate"
import * as Types from "../../api/types"
import { errorString } from "../../util/error"

export interface AgentContext {
  agentId?: string
  netstat?: Types.NetstatResponse
  websocket?: WebSocket
}

export type AgentEvent =
  | { type: "CONNECT"; agentId: string }
  | { type: "STAT"; data: Types.NetstatResponse }
  | { type: "DISCONNECT" }

export const agentMachine = createMachine(
  {
    tsTypes: {} as import("./agentXService.typegen").Typegen0,
    schema: {
      context: {} as AgentContext,
      events: {} as AgentEvent,
      services: {} as {
        connect: {
          data: WebSocket
        }
      },
    },
    id: "agentState",
    initial: "disconnected",
    states: {
      connecting: {
        invoke: {
          src: "connect",
          id: "connect",
          onDone: [
            {
              actions: ["assignWebsocket", "clearNetstat"],
              target: "connected",
            },
          ],
          onError: [
            {
              actions: "assignWebsocketError",
              target: "disconnected",
            },
          ],
        },
      },
      connected: {
        on: {
          STAT: {
            actions: "assignNetstat",
          },
          DISCONNECT: {
            actions: ["disconnect", "clearNetstat"],
            target: "disconnected",
          },
        },
      },
      disconnected: {
        on: {
          CONNECT: {
            actions: "assignConnection",
            target: "connecting",
          },
        },
      },
    },
  },
  {
    services: {
      connect: (context) => (send) => {
        return new Promise<WebSocket>((resolve, reject) => {
          if (!context.agentId) {
            return reject("agent ID is not set")
          }
          const proto = location.protocol === "https:" ? "wss:" : "ws:"
          const socket = new WebSocket(`${proto}//${location.host}/api/v2/workspaceagents/${context.agentId}/netstat`)
          socket.binaryType = "arraybuffer"
          socket.addEventListener("open", () => {
            resolve(socket)
          })
          socket.addEventListener("error", (error) => {
            reject(error)
          })
          socket.addEventListener("close", () => {
            send({
              type: "DISCONNECT",
            })
          })
          socket.addEventListener("message", (event) => {
            try {
              send({
                type: "STAT",
                data: JSON.parse(new TextDecoder().decode(event.data)),
              })
            } catch (error) {
              send({
                type: "STAT",
                data: {
                  error: errorString(error),
                },
              })
            }
          })
        })
      },
    },
    actions: {
      assignConnection: assign((context, event) => ({
        ...context,
        agentId: event.agentId,
      })),
      assignWebsocket: assign({
        websocket: (_, event) => event.data,
      }),
      assignWebsocketError: assign({
        netstat: (_, event) => ({ error: errorString(event.data) }),
      }),
      clearNetstat: assign((context: AgentContext) => ({
        ...context,
        netstat: undefined,
      })),
      assignNetstat: assign({
        netstat: (_, event) => event.data,
      }),
      disconnect: (context: AgentContext) => {
        // Code 1000 is a successful exit!
        context.websocket?.close(1000)
      },
    },
  },
)
