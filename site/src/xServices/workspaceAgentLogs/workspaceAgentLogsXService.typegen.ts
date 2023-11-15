// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.streamLogs": {
      type: "done.invoke.streamLogs";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.workspaceAgentLogsMachine.loading:invocation[0]": {
      type: "done.invoke.workspaceAgentLogsMachine.loading:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.streamLogs": {
      type: "error.platform.streamLogs";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    getLogs: "done.invoke.workspaceAgentLogsMachine.loading:invocation[0]";
    streamLogs: "done.invoke.streamLogs";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    addLogs: "ADD_LOGS";
    assignLogs: "done.invoke.workspaceAgentLogsMachine.loading:invocation[0]";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {
    getLogs: "FETCH_LOGS";
    streamLogs: "done.invoke.workspaceAgentLogsMachine.loading:invocation[0]";
  };
  matchesStates: "loaded" | "loading" | "waiting" | "watchLogs";
  tags: never;
}
