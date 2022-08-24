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
    "done.invoke.checkPermissions": {
      type: "done.invoke.checkPermissions"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.checkPermissions": { type: "error.platform.checkPermissions"; data: unknown }
    "done.invoke.submitSchedule": {
      type: "done.invoke.submitSchedule"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.submitSchedule": { type: "error.platform.submitSchedule"; data: unknown }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    getWorkspace: "done.invoke.getWorkspace"
    checkPermissions: "done.invoke.checkPermissions"
    submitSchedule: "done.invoke.submitSchedule"
  }
  missingImplementations: {
    actions: never
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignWorkspace: "done.invoke.getWorkspace"
    assignGetWorkspaceError: "error.platform.getWorkspace"
    assignPermissions: "done.invoke.checkPermissions"
    assignGetPermissionsError: "error.platform.checkPermissions"
    displaySuccess: "done.invoke.submitSchedule"
    assignSubmissionError: "error.platform.submitSchedule"
    clearGetWorkspaceError: "GET_WORKSPACE"
    clearContext: "GET_WORKSPACE"
    clearGetPermissionsError: "done.invoke.getWorkspace"
  }
  eventsCausingServices: {
    getWorkspace: "GET_WORKSPACE"
    checkPermissions: "done.invoke.getWorkspace"
    submitSchedule: "SUBMIT_SCHEDULE"
  }
  eventsCausingGuards: {}
  eventsCausingDelays: {}
  matchesStates:
    | "idle"
    | "gettingWorkspace"
    | "gettingPermissions"
    | "presentForm"
    | "submittingSchedule"
    | "submitSuccess"
    | "error"
  tags: "loading"
}
