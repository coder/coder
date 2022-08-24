// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.createFirstUser": {
      type: "done.invoke.createFirstUser"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.createFirstUser": { type: "error.platform.createFirstUser"; data: unknown }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    createFirstUser: "done.invoke.createFirstUser"
  }
  missingImplementations: {
    actions: "onCreateFirstUser"
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignFirstUserData: "CREATE_FIRST_USER"
    onCreateFirstUser: "done.invoke.createFirstUser"
    assignCreateFirstUserFormErrors: "error.platform.createFirstUser"
    assignCreateFirstUserError: "error.platform.createFirstUser"
    clearCreateFirstUserError: "CREATE_FIRST_USER"
  }
  eventsCausingServices: {
    createFirstUser: "CREATE_FIRST_USER"
  }
  eventsCausingGuards: {
    hasFieldErrors: "error.platform.createFirstUser"
  }
  eventsCausingDelays: {}
  matchesStates: "idle" | "creatingFirstUser" | "firstUserCreated"
  tags: "loading"
}
