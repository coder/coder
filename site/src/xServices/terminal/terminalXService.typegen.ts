// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.connect": {
      type: "done.invoke.connect";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.getWebsocketURL": {
      type: "done.invoke.getWebsocketURL";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.getWorkspace": {
      type: "done.invoke.getWorkspace";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.getWorkspaceAgent": {
      type: "done.invoke.getWorkspaceAgent";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.connect": { type: "error.platform.connect"; data: unknown };
    "error.platform.getWebsocketURL": {
      type: "error.platform.getWebsocketURL";
      data: unknown;
    };
    "error.platform.getWorkspace": {
      type: "error.platform.getWorkspace";
      data: unknown;
    };
    "error.platform.getWorkspaceAgent": {
      type: "error.platform.getWorkspaceAgent";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    connect: "done.invoke.connect";
    getWebsocketURL: "done.invoke.getWebsocketURL";
    getWorkspace: "done.invoke.getWorkspace";
    getWorkspaceAgent: "done.invoke.getWorkspaceAgent";
  };
  missingImplementations: {
    actions: "readMessage";
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignConnection: "CONNECT";
    assignWebsocket: "done.invoke.connect";
    assignWebsocketError: "error.platform.connect";
    assignWebsocketURL: "done.invoke.getWebsocketURL";
    assignWebsocketURLError: "error.platform.getWebsocketURL";
    assignWorkspace: "done.invoke.getWorkspace";
    assignWorkspaceAgent: "done.invoke.getWorkspaceAgent";
    assignWorkspaceAgentError: "error.platform.getWorkspaceAgent";
    assignWorkspaceError: "error.platform.getWorkspace";
    clearWebsocketError: "done.invoke.connect";
    clearWebsocketURLError: "done.invoke.getWebsocketURL";
    clearWorkspaceAgentError: "done.invoke.getWorkspaceAgent";
    clearWorkspaceError: "done.invoke.getWorkspace";
    disconnect: "DISCONNECT";
    readMessage: "READ";
    sendMessage: "WRITE";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {
    connect: "done.invoke.getWebsocketURL";
    getWebsocketURL: "done.invoke.getWorkspaceAgent";
    getWorkspace: "xstate.init";
    getWorkspaceAgent: "CONNECT" | "done.state.terminalState.setup";
  };
  matchesStates:
    | "connected"
    | "connecting"
    | "disconnected"
    | "gettingWebSocketURL"
    | "gettingWorkspaceAgent"
    | "setup"
    | "setup.getWorkspace"
    | "setup.getWorkspace.gettingWorkspace"
    | "setup.getWorkspace.success"
    | {
        setup?:
          | "getWorkspace"
          | { getWorkspace?: "gettingWorkspace" | "success" };
      };
  tags: never;
}
