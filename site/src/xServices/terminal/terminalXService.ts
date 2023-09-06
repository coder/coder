import { assign, createMachine } from "xstate";
import * as API from "../../api/api";
import * as Types from "../../api/types";
import * as TypesGen from "../../api/typesGenerated";

export interface TerminalContext {
  workspaceError?: unknown;
  workspace?: TypesGen.Workspace;
  workspaceAgent?: TypesGen.WorkspaceAgent;
  workspaceAgentError?: unknown;
  websocket?: WebSocket;
  websocketError?: unknown;
  websocketURL?: string;
  websocketURLError?: unknown;

  // Assigned by connecting!
  // The workspace agent is entirely optional.  If the agent is omitted the
  // first agent will be used.
  agentName?: string;
  username?: string;
  workspaceName?: string;
  reconnection?: string;
  command?: string;
  // If baseURL is not.....
  baseURL?: string;
}

export type TerminalEvent =
  | {
      type: "CONNECT";
      agentName?: string;
      reconnection?: string;
      workspaceName?: string;
      username?: string;
    }
  | { type: "WRITE"; request: Types.ReconnectingPTYRequest }
  | { type: "READ"; data: ArrayBuffer }
  | { type: "DISCONNECT" };

export const terminalMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QBcwCcC2BLAdgQwBsBlZPVAOljGQFcAHAYggHscxLSLVNdCSz2VWnQDaABgC6iUHWawsyLK2kgAHogAcAVgCM5MQGYdAJgMaALOYDsV8zq0AaEAE9ExgGwHyATmPf3VmI6Yub+5gYAvhFO3Nj4xJyC1PTkMMgA6sxoANawdHgAxuxpijhQmTl5hWBMrOy4AG7M2cXUFbn5ReJSSCCy8orKveoI5qbkOhq23hozflruxuZOrghGXuZaRsFiWlbeWuYa7lEx6HF8iZTJdKltWR3Vd8il5Q9VRQzoaFnkdARkABmWQwz3aHzA3RU-QUShwKhGYy8k2msw080WyxcbmMYnIoW8hO8ZkMYisOlOIFivASAmer3BnTAAEEYDhkLU2ORGs1Whl3kzWWB2VDejDBvDhogdAYrFoJuYxJjdmJvCENCs3Fo8e4glpjFptWSDhpKdT4vwKCVcG9KoK2Rzvr9-kCQWCBdUhSLJNC5LChqARotNQgpniNAYtAdjFYDL4pidolTzjTLXyGWAAEZEZgFFrIACqACUADKc+o4JotZ45vPUYsl0UyP0ShGIWVWchGA27Akzbwh4LjDQmAk6AmBbxmlMWq7WsrpLO1-MNr5oH5oP4A5DAzA13Mr0tNvotuFthBWYyDo56ULGewWHTk0zGac8Wd0gqsNgFV7l7mVry5BfjgP7IMe4pnlKobmO45D3mG7ihFMBgBIOhgaPoizar4xIRv4b4XLSFAgWBNprhuW6unupFgL+EGngGaiIMG2IIPYtj4psMYaGIxh+Do9iEamVy0b+kAMOkRYAJIACoAKIMQMUGBogdiYZs3Z+N444GqYg42F4kY6LqmxWLMUaREm5qXJ+350agEAMEW8nMgAIkp-qSqpow6N45BIRYkYRgsBxyoOASdnG2gyu43jcUhwkfiR9niU5bnSUQADCADyAByeXyVlsmea20F+Zho6EksgU6WIGpsZMuj6OZfimLB0bmEltkUBAWCwGJjkMLlBVFSVPpiox3nMexV6Na+lI4MwEBwCoNnEWAvrKUxIwALRjCGu1yuQRpiCEBpyrshLdRt1zCFtXnnu4IabCdZ1nWMezalGFLWTOPVJMI7p2tUD1lT5hyDgYXjvR9KrxSZXV-e+AN3SkaSMk8862o8RRgypM1DiGL4+FY7iGvqniXvYSNnCjt1COj9wg0UlA0AURSwPAk3bdNQbQ-o3YLCYHGwcTRwBeE-GYjToRWDdab0jamNFF6yD4ztiD+PKgTdtYljuAEBjE3G+J+HFI7ah45IK3O1AZtmB71qWGt84gezofe+j2Lq7h+XxSFWXTRGK4NNqu09fFYUssyLPxCyOI1hjyh4CzW1b3jy8jIeialjkR9BhzweTiwBPqOm2Asg5TOYXbqmY6xISEtt0n1A155ABc+aheiGCYfhGAnthWIO33kOSxwRn3vgBFEURAA */
  createMachine(
    {
      id: "terminalState",
      predictableActionArguments: true,
      tsTypes: {} as import("./terminalXService.typegen").Typegen0,
      schema: {
        context: {} as TerminalContext,
        events: {} as TerminalEvent,
        services: {} as {
          getWorkspace: {
            data: TypesGen.Workspace;
          };
          getWorkspaceAgent: {
            data: TypesGen.WorkspaceAgent;
          };
          getWebsocketURL: {
            data: string;
          };
          connect: {
            data: WebSocket;
          };
        },
      },
      initial: "setup",
      states: {
        setup: {
          type: "parallel",
          states: {
            getWorkspace: {
              initial: "gettingWorkspace",
              states: {
                gettingWorkspace: {
                  invoke: {
                    src: "getWorkspace",
                    id: "getWorkspace",
                    onDone: [
                      {
                        actions: ["assignWorkspace", "clearWorkspaceError"],
                        target: "success",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignWorkspaceError",
                        target: "success",
                      },
                    ],
                  },
                },
                success: {
                  type: "final",
                },
              },
            },
          },
          onDone: {
            target: "gettingWorkspaceAgent",
          },
        },
        gettingWorkspaceAgent: {
          invoke: {
            src: "getWorkspaceAgent",
            id: "getWorkspaceAgent",
            onDone: [
              {
                actions: ["assignWorkspaceAgent", "clearWorkspaceAgentError"],
                target: "gettingWebSocketURL",
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
        gettingWebSocketURL: {
          invoke: {
            src: "getWebsocketURL",
            id: "getWebsocketURL",
            onDone: [
              {
                actions: ["assignWebsocketURL", "clearWebsocketURLError"],
                target: "connecting",
              },
            ],
            onError: [
              {
                actions: "assignWebsocketURLError",
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
              target: "gettingWorkspaceAgent",
            },
          },
        },
      },
    },
    {
      services: {
        getWorkspace: async (context) => {
          if (!context.workspaceName) {
            throw new Error("workspace name not set");
          }
          return API.getWorkspaceByOwnerAndName(
            context.username,
            context.workspaceName,
          );
        },
        getWorkspaceAgent: async (context) => {
          if (!context.workspace || !context.workspaceName) {
            throw new Error("workspace or workspace name is not set");
          }

          const agent = context.workspace.latest_build.resources
            .map((resource) => {
              if (!resource.agents || resource.agents.length === 0) {
                return;
              }
              if (!context.agentName) {
                return resource.agents[0];
              }
              return resource.agents.find(
                (agent) => agent.name === context.agentName,
              );
            })
            .filter((a) => a)[0];
          if (!agent) {
            throw new Error("no agent found with id");
          }
          return agent;
        },
        getWebsocketURL: async (context) => {
          if (!context.workspaceAgent) {
            throw new Error("workspace agent is not set");
          }
          if (!context.reconnection) {
            throw new Error("reconnection ID is not set");
          }

          let baseURL = context.baseURL || "";
          if (!baseURL) {
            baseURL = `${location.protocol}//${location.host}`;
          }

          const query = new URLSearchParams({
            reconnect: context.reconnection,
          });
          if (context.command) {
            query.set("command", context.command);
          }

          const url = new URL(baseURL);
          url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
          if (!url.pathname.endsWith("/")) {
            url.pathname + "/";
          }
          url.pathname += `api/v2/workspaceagents/${context.workspaceAgent.id}/pty`;
          url.search = "?" + query.toString();

          // If the URL is just the primary API, we don't need a signed token to
          // connect.
          if (!context.baseURL) {
            return url.toString();
          }

          // Do ticket issuance and set the query parameter.
          const tokenRes = await API.issueReconnectingPTYSignedToken({
            url: url.toString(),
            agentID: context.workspaceAgent.id,
          });
          query.set("coder_signed_app_token_23db1dde", tokenRes.signed_token);
          url.search = "?" + query.toString();

          return url.toString();
        },
        connect: (context) => (send) => {
          return new Promise<WebSocket>((resolve, reject) => {
            if (!context.workspaceAgent) {
              return reject("workspace agent is not set");
            }
            if (!context.websocketURL) {
              return reject("websocket URL is not set");
            }

            const socket = new WebSocket(context.websocketURL);
            socket.binaryType = "arraybuffer";
            socket.addEventListener("open", () => {
              resolve(socket);
            });
            socket.addEventListener("error", () => {
              reject(new Error("socket errored"));
            });
            socket.addEventListener("close", () => {
              send({
                type: "DISCONNECT",
              });
            });
            socket.addEventListener("message", (event) => {
              send({
                type: "READ",
                data: event.data,
              });
            });
          });
        },
      },
      actions: {
        assignConnection: assign((context, event) => ({
          ...context,
          agentName: event.agentName ?? context.agentName,
          reconnection: event.reconnection ?? context.reconnection,
          workspaceName: event.workspaceName ?? context.workspaceName,
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
        assignWebsocketURL: assign({
          websocketURL: (context, event) => event.data ?? context.websocketURL,
        }),
        assignWebsocketURLError: assign({
          websocketURLError: (_, event) => event.data,
        }),
        clearWebsocketURLError: assign((context: TerminalContext) => ({
          ...context,
          websocketURLError: undefined,
        })),
        sendMessage: (context, event) => {
          if (!context.websocket) {
            throw new Error("websocket doesn't exist");
          }
          context.websocket.send(
            new TextEncoder().encode(JSON.stringify(event.request)),
          );
        },
        disconnect: (context: TerminalContext) => {
          // Code 1000 is a successful exit!
          context.websocket?.close(1000);
        },
      },
    },
  );
