// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.getRoles": {
      type: "done.invoke.getRoles"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getRoles": { type: "error.platform.getRoles"; data: unknown }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    getRoles: "done.invoke.getRoles"
  }
  missingImplementations: {
    actions: never
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignRoles: "done.invoke.getRoles"
    assignGetRolesError: "error.platform.getRoles"
    displayGetRolesError: "error.platform.getRoles"
    clearGetRolesError: "GET_ROLES"
  }
  eventsCausingServices: {
    getRoles: "GET_ROLES"
  }
  eventsCausingGuards: {}
  eventsCausingDelays: {}
  matchesStates: "idle" | "gettingRoles"
  tags: never
}
