// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.getEntitlements": {
      type: "done.invoke.getEntitlements"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getEntitlements": { type: "error.platform.getEntitlements"; data: unknown }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    getEntitlements: "done.invoke.getEntitlements"
  }
  missingImplementations: {
    actions: never
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignMockEntitlements: "SHOW_MOCK_BANNER"
    clearMockEntitlements: "HIDE_MOCK_BANNER"
    assignEntitlements: "done.invoke.getEntitlements"
    assignGetEntitlementsError: "error.platform.getEntitlements"
    clearGetEntitlementsError: "GET_ENTITLEMENTS"
  }
  eventsCausingServices: {
    getEntitlements: "GET_ENTITLEMENTS"
  }
  eventsCausingGuards: {}
  eventsCausingDelays: {}
  matchesStates: "idle" | "gettingEntitlements"
  tags: never
}
