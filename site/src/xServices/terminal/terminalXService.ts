import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"

export interface TerminalContext {
  organizationsError?: Error | unknown
  organizations?: Types.Organization[]
  workspaceError?: Error | unknown
  workspace?: Types.Workspace
  workspaceAgent?: Types.WorkspaceAgent
  workspaceAgentError?: Error | unknown
  websocket?: WebSocket
  websocketError?: Error | unknown

  // Assigned by connecting!
  username?: string
  workspaceName?: string
  reconnection?: string
}

export type TerminalEvent =
  | { type: "CONNECT"; reconnection?: string; workspaceName?: string; username?: string }
  | { type: "WRITE"; request: Types.ReconnectingPTYRequest }
  | { type: "READ"; data: ArrayBuffer }
  | { type: "DISCONNECT" }

export const terminalMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QBcwCcC2BLAdgQwBsBlZPVAOhmWVygHk0o8csAvMrAex1gGIJuYcrgBunANZCqDJi3Y1u8JCAAOnWFgU5EoAB6IA7AE4AzOSMBGAEwBWCyYAsANgs2bVgDQgAnohNGbcgsnEIcHCwAOBwAGCKMHAF8Er1RMXEISMikwaloZZjYORV50NE40chUCMgAzcoxKHPy5Ip4dVXVNLm1lfQQIiLMIpxMbQYjYo2jXL18EawtyAwGwk1HTMdGklPRsfGJSCioaHCgAdXLxWBU8AGMwfkFhHDFJRuQLtCub+-a1DS07T69icVnILisDhspiccQ2sz8y3INmiqNGsMcBmiNm2IFSewyh2yuVOn2+dwepXKlWqyDqmHeZOuFL+nUBvUQILBEKhMLhowRCAMTkCIzWDkcEyMBhsBlx+PSByy7xO50uzPuAEEYDhkI8cEJRBJiUyfmBtWBdayAd0gZyjCFyFZbKjnC4jKYLIK7GYYqirCYBk5olYDIlknjdorMkccqrTRSLbqSmgyhUqrV6oz1Wak8hrV1uHb5g6nE6XdE3RYPSYvT5EWCA2s7E4sbZhfKo-sY0JbtwDbdVfrDS9jeQ+zgB-nlP9Cz09IhIcLyCZIUYotEDCZNxEDN7peY1ms4gYrNNBp20t2ieP+2BB7QU2maZmGROpwX2QuEEuy6uHOuMRbjue71ggdiLPY4phiYMoWNYl4EkqFDvveqAQLwZwAEoAJIACoAKKfraHI-gYkTkCGAFYmG0TbnWcw2A4YKmGskJno44RWIh0Y3qhg6QLwWEEZqAAixFFqRobViusKngYMpWEGgpQmWUFsSEcSOHRPHXsq-Hobwok4UQADCdAAHIWQRpl4RJ84gH0SmtuQDgTNWMJTDKJgqUEq4RNCljTOs3ERgqekUBAWCwAZgnmVZNl2TObIkd+zplrEYanvY0SwrCESCs6BiuaidGrgGTgekYSQRjgnAQHA7ThYSyrHHkjAFPI3RKKAs5fo5iAxIEXHMSMErWNiTiFa4K6ldi1ayu4FjhjsV4tbGJJql8GpgPZxYWNMDiubBdh2Ju7pGIK-jFW6rYiq4bZOLp63EvGOaJjq069Slknfq4jrOii51udiMpXbC5ATKi1ghNEljNs9yG9neD6nHtUlnhErmgqeIyosKazejNZ7+qMi3BiYiM9rek5oZA6NpeR0ROmGpgnhT0qCmNSxHhKW4BiGERUzeUUxSj6EMwN4HM5ukLLXBViRKGNhXVj64etM0JOG5YTC1kktORlp7hA4CtK2DYEALQQ8M5FKQd27OJTNVAA */
  createMachine(
    {
      tsTypes: {} as import("./terminalXService.typegen").Typegen0,
      schema: {
        context: {} as TerminalContext,
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
                target: "disconnected",
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
                target: "disconnected",
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
                target: "disconnected",
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
                target: "disconnected",
              },
            ],
          },
        },
        connected: {
          on: {
            WRITE: {
              actions: "sendMessage",
            },
            READ: {
              actions: "readMessage",
            },
            DISCONNECT: {
              actions: "disconnect",
              target: "disconnected",
            },
          },
        },
        disconnected: {
          on: {
            CONNECT: {
              actions: "assignConnection",
              target: "gettingOrganizations",
            },
          },
        },
      },
    },
    {
      services: {
        getOrganizations: API.getOrganizations,
        getWorkspace: async (context) => {
          if (!context.organizations || !context.workspaceName) {
            throw new Error("organizations or workspace not set")
          }
          return API.getWorkspace(context.organizations[0].id, context.username, context.workspaceName)
        },
        getWorkspaceAgent: async (context) => {
          if (!context.workspace || !context.workspaceName) {
            throw new Error("workspace or workspace name is not set")
          }
          // The workspace name is in the format:
          // <workspace name>[.<agent name>]
          // The workspace agent is entirely optional.
          const workspaceNameParts = context.workspaceName.split(".")
          const agentName = workspaceNameParts[1]

          const resources = await API.getWorkspaceResources(context.workspace.latest_build.id)

          const agent = resources
            .map((resource) => {
              if (!resource.agents || resource.agents.length < 1) {
                return
              }
              if (!agentName) {
                return resource.agents[0]
              }
              return resource.agents.find((agent) => agent.name === agentName)
            })
            .filter((a) => a)[0]
          if (!agent) {
            throw new Error("no agent found with id")
          }
          return agent
        },
        connect: (context) => (send) => {
          return new Promise<WebSocket>((resolve, reject) => {
            if (!context.workspaceAgent) {
              return reject("workspace agent is not set")
            }
            const proto = location.protocol === "https:" ? "wss:" : "ws:"
            const socket = new WebSocket(
              `${proto}//${location.host}/api/v2/workspaceagents/${context.workspaceAgent.id}/pty?reconnect=${context.reconnection}`,
            )
            socket.binaryType = "arraybuffer"
            socket.addEventListener("open", () => {
              resolve(socket)
            })
            socket.addEventListener("error", () => {
              reject(new Error("socket errored"))
            })
            socket.addEventListener("close", () => {
              send({
                type: "DISCONNECT",
              })
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
        assignConnection: assign((context, event) => ({
          ...context,
          reconnection: event.reconnection ?? context.reconnection,
          workspaceName: event.workspaceName ?? context.workspaceName,
        })),
        assignOrganizations: assign({
          organizations: (_, event) => event.data,
        }),
        assignOrganizationsError: assign({
          organizationsError: (_, event) => event.data,
        }),
        clearOrganizationsError: assign((context) => ({
          ...context,
          organizationsError: undefined,
        })),
        assignWorkspace: assign({
          workspace: (_, event) => event.data,
        }),
        assignWorkspaceError: assign({
          workspaceError: (_, event) => event.data,
        }),
        clearWorkspaceError: assign((context) => ({
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
        assignWebsocket: assign({
          websocket: (_, event) => event.data,
        }),
        assignWebsocketError: assign({
          websocketError: (_, event) => event.data,
        }),
        clearWebsocketError: assign((context: TerminalContext) => ({
          ...context,
          webSocketError: undefined,
        })),
        sendMessage: (context, event) => {
          if (!context.websocket) {
            throw new Error("websocket doesn't exist")
          }
          context.websocket.send(new TextEncoder().encode(JSON.stringify(event.request)))
        },
        disconnect: (context: TerminalContext) => {
          // Code 1000 is a successful exit!
          context.websocket?.close(1000)
        },
      },
    },
  )
