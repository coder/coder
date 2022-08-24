// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.updateDeadline": {
      type: "done.invoke.updateDeadline"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.updateDeadline": { type: "error.platform.updateDeadline"; data: unknown }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    updateDeadline: "done.invoke.updateDeadline"
  }
  missingImplementations: {
    actions: never
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    displaySuccessMessage: "done.invoke.updateDeadline"
    displayFailureMessage: "error.platform.updateDeadline"
  }
  eventsCausingServices: {
    updateDeadline: "UPDATE_DEADLINE"
  }
  eventsCausingGuards: {}
  eventsCausingDelays: {}
  matchesStates: "idle" | "updatingDeadline"
  tags: "loading"
}
