// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.workspaceBuildState.gettingBuild:invocation[0]": {
      type: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.workspaceBuildState.gettingBuild:invocation[0]": {
      type: "error.platform.workspaceBuildState.gettingBuild:invocation[0]"
      data: unknown
    }
    "done.invoke.getLogs": {
      type: "done.invoke.getLogs"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "xstate.init": { type: "xstate.init" }
    "error.platform.getLogs": { type: "error.platform.getLogs"; data: unknown }
    "done.invoke.streamWorkspaceBuildLogs": {
      type: "done.invoke.streamWorkspaceBuildLogs"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.streamWorkspaceBuildLogs": {
      type: "error.platform.streamWorkspaceBuildLogs"
      data: unknown
    }
  }
  invokeSrcNameMap: {
    getWorkspaceBuild: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]"
    getLogs: "done.invoke.getLogs"
    streamWorkspaceBuildLogs: "done.invoke.streamWorkspaceBuildLogs"
  }
  missingImplementations: {
    actions: never
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignBuild: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]"
    assignBuildId: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]"
    assignGetBuildError: "error.platform.workspaceBuildState.gettingBuild:invocation[0]"
    addLog: "ADD_LOG"
    assignLogs: "done.invoke.getLogs"
    clearGetBuildError: "xstate.init"
  }
  eventsCausingServices: {
    getWorkspaceBuild: "xstate.init"
    getLogs: "done.invoke.workspaceBuildState.gettingBuild:invocation[0]"
    streamWorkspaceBuildLogs: "done.invoke.getLogs"
  }
  eventsCausingGuards: {}
  eventsCausingDelays: {}
  matchesStates:
    | "gettingBuild"
    | "idle"
    | "logs"
    | "logs.gettingExistentLogs"
    | "logs.watchingLogs"
    | "logs.loaded"
    | { logs?: "gettingExistentLogs" | "watchingLogs" | "loaded" }
  tags: never
}
