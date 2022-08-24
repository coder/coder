// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.getOrganizations": {
      type: "done.invoke.getOrganizations"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "xstate.init": { type: "xstate.init" }
    "error.platform.getOrganizations": { type: "error.platform.getOrganizations"; data: unknown }
    "done.invoke.getTemplates": {
      type: "done.invoke.getTemplates"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getTemplates": { type: "error.platform.getTemplates"; data: unknown }
  }
  invokeSrcNameMap: {
    getOrganizations: "done.invoke.getOrganizations"
    getTemplates: "done.invoke.getTemplates"
  }
  missingImplementations: {
    actions: never
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignOrganizations: "done.invoke.getOrganizations"
    clearOrganizationsError: "done.invoke.getOrganizations" | "xstate.init"
    assignOrganizationsError: "error.platform.getOrganizations"
    assignTemplates: "done.invoke.getTemplates"
    clearTemplatesError: "done.invoke.getTemplates" | "done.invoke.getOrganizations"
    assignTemplatesError: "error.platform.getTemplates"
  }
  eventsCausingServices: {
    getOrganizations: "xstate.init"
    getTemplates: "done.invoke.getOrganizations"
  }
  eventsCausingGuards: {}
  eventsCausingDelays: {}
  matchesStates: "gettingOrganizations" | "gettingTemplates" | "done" | "error"
  tags: "loading"
}
