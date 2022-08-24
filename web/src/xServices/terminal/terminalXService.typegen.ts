// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.getWorkspace": {
      type: "done.invoke.getWorkspace"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getWorkspace": { type: "error.platform.getWorkspace"; data: unknown }
    "done.invoke.getWorkspaceAgent": {
      type: "done.invoke.getWorkspaceAgent"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getWorkspaceAgent": { type: "error.platform.getWorkspaceAgent"; data: unknown }
    "done.invoke.connect": {
      type: "done.invoke.connect"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.connect": { type: "error.platform.connect"; data: unknown }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    getWorkspace: "done.invoke.getWorkspace"
    getWorkspaceAgent: "done.invoke.getWorkspaceAgent"
    connect: "done.invoke.connect"
  }
  missingImplementations: {
    actions: "readMessage"
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignWorkspace: "done.invoke.getWorkspace"
    clearWorkspaceError: "done.invoke.getWorkspace"
    assignWorkspaceError: "error.platform.getWorkspace"
    assignWorkspaceAgent: "done.invoke.getWorkspaceAgent"
    clearWorkspaceAgentError: "done.invoke.getWorkspaceAgent"
    assignWorkspaceAgentError: "error.platform.getWorkspaceAgent"
    assignWebsocket: "done.invoke.connect"
    clearWebsocketError: "done.invoke.connect"
    assignWebsocketError: "error.platform.connect"
    sendMessage: "WRITE"
    readMessage: "READ"
    disconnect: "DISCONNECT"
    assignConnection: "CONNECT"
  }
  eventsCausingServices: {
    getWorkspace: "xstate.init" | "CONNECT"
    getWorkspaceAgent: "done.invoke.getWorkspace"
    connect: "done.invoke.getWorkspaceAgent"
  }
  eventsCausingGuards: {}
  eventsCausingDelays: {}
  matchesStates:
    | "gettingWorkspace"
    | "gettingWorkspaceAgent"
    | "connecting"
    | "connected"
    | "disconnected"
  tags: never
}
