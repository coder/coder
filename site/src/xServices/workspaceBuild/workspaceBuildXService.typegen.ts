// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.getLogs": {
      type: "done.invoke.getLogs";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.streamWorkspaceBuildLogs": {
      type: "done.invoke.streamWorkspaceBuildLogs";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.workspaceBuildState.gettingBuild:invocation[0]": {
      type: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.getLogs": { type: "error.platform.getLogs"; data: unknown };
    "error.platform.streamWorkspaceBuildLogs": {
      type: "error.platform.streamWorkspaceBuildLogs";
      data: unknown;
    };
    "error.platform.workspaceBuildState.gettingBuild:invocation[0]": {
      type: "error.platform.workspaceBuildState.gettingBuild:invocation[0]";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    getLogs: "done.invoke.getLogs";
    getWorkspaceBuild: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]";
    streamWorkspaceBuildLogs: "done.invoke.streamWorkspaceBuildLogs";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    addLog: "ADD_LOG";
    assignBuild: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]";
    assignBuildId: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]";
    assignGetBuildError: "error.platform.workspaceBuildState.gettingBuild:invocation[0]";
    assignLogs: "done.invoke.getLogs";
    clearGetBuildError: "RESET" | "xstate.init";
    resetContext: "RESET";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {
    getLogs: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]";
    getWorkspaceBuild: "RESET" | "xstate.init";
    streamWorkspaceBuildLogs: "done.invoke.getLogs";
  };
  matchesStates:
    | "gettingBuild"
    | "idle"
    | "logs"
    | "logs.gettingExistentLogs"
    | "logs.loaded"
    | "logs.watchingLogs"
    | { logs?: "gettingExistentLogs" | "loaded" | "watchingLogs" };
  tags: never;
}
