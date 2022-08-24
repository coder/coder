// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.getWorkspace": {
      type: "done.invoke.getWorkspace"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.refreshWorkspace": {
      type: "done.invoke.refreshWorkspace"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getWorkspace": { type: "error.platform.getWorkspace"; data: unknown }
    "done.invoke.refreshTemplate": {
      type: "done.invoke.refreshTemplate"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.refreshTemplate": { type: "error.platform.refreshTemplate"; data: unknown }
    "done.invoke.checkPermissions": {
      type: "done.invoke.checkPermissions"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.checkPermissions": { type: "error.platform.checkPermissions"; data: unknown }
    "done.invoke.startWorkspace": {
      type: "done.invoke.startWorkspace"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.stopWorkspace": {
      type: "done.invoke.stopWorkspace"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.deleteWorkspace": {
      type: "done.invoke.deleteWorkspace"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.cancelWorkspace": {
      type: "done.invoke.cancelWorkspace"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.refreshWorkspace": { type: "error.platform.refreshWorkspace"; data: unknown }
    "xstate.after(1000)#workspaceState.ready.pollingWorkspace.waiting": {
      type: "xstate.after(1000)#workspaceState.ready.pollingWorkspace.waiting"
    }
    "xstate.after(5000)#workspaceState.ready.pollingResources.waiting": {
      type: "xstate.after(5000)#workspaceState.ready.pollingResources.waiting"
    }
    "error.platform.startWorkspace": { type: "error.platform.startWorkspace"; data: unknown }
    "error.platform.stopWorkspace": { type: "error.platform.stopWorkspace"; data: unknown }
    "error.platform.deleteWorkspace": { type: "error.platform.deleteWorkspace"; data: unknown }
    "error.platform.cancelWorkspace": { type: "error.platform.cancelWorkspace"; data: unknown }
    "done.invoke.getResources": {
      type: "done.invoke.getResources"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getResources": { type: "error.platform.getResources"; data: unknown }
    "done.invoke.workspaceState.ready.timeline.gettingBuilds:invocation[0]": {
      type: "done.invoke.workspaceState.ready.timeline.gettingBuilds:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.workspaceState.ready.timeline.gettingBuilds:invocation[0]": {
      type: "error.platform.workspaceState.ready.timeline.gettingBuilds:invocation[0]"
      data: unknown
    }
    "done.invoke.workspaceState.ready.timeline.loadedBuilds.loadingMoreBuilds:invocation[0]": {
      type: "done.invoke.workspaceState.ready.timeline.loadedBuilds.loadingMoreBuilds:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.workspaceState.ready.timeline.loadedBuilds.loadingMoreBuilds:invocation[0]": {
      type: "error.platform.workspaceState.ready.timeline.loadedBuilds.loadingMoreBuilds:invocation[0]"
      data: unknown
    }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    getWorkspace: "done.invoke.getWorkspace"
    getTemplate: "done.invoke.refreshTemplate"
    checkPermissions: "done.invoke.checkPermissions"
    refreshWorkspace: "done.invoke.refreshWorkspace"
    startWorkspace: "done.invoke.startWorkspace"
    stopWorkspace: "done.invoke.stopWorkspace"
    deleteWorkspace: "done.invoke.deleteWorkspace"
    cancelWorkspace: "done.invoke.cancelWorkspace"
    getResources: "done.invoke.getResources"
    getBuilds: "done.invoke.workspaceState.ready.timeline.gettingBuilds:invocation[0]"
    loadMoreBuilds: "done.invoke.workspaceState.ready.timeline.loadedBuilds.loadingMoreBuilds:invocation[0]"
  }
  missingImplementations: {
    actions: never
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignWorkspace: "done.invoke.getWorkspace" | "done.invoke.refreshWorkspace"
    assignGetWorkspaceError: "error.platform.getWorkspace"
    assignTemplate: "done.invoke.refreshTemplate"
    assignRefreshTemplateError: "error.platform.refreshTemplate"
    displayRefreshTemplateError: "error.platform.refreshTemplate"
    assignPermissions: "done.invoke.checkPermissions"
    assignGetPermissionsError: "error.platform.checkPermissions"
    refreshTimeline:
      | "done.invoke.refreshWorkspace"
      | "done.invoke.startWorkspace"
      | "done.invoke.stopWorkspace"
      | "done.invoke.deleteWorkspace"
      | "done.invoke.cancelWorkspace"
    assignRefreshWorkspaceError: "error.platform.refreshWorkspace"
    assignBuild:
      | "done.invoke.startWorkspace"
      | "done.invoke.stopWorkspace"
      | "done.invoke.deleteWorkspace"
    assignBuildError:
      | "error.platform.startWorkspace"
      | "error.platform.stopWorkspace"
      | "error.platform.deleteWorkspace"
    assignCancellationMessage: "done.invoke.cancelWorkspace"
    displayCancellationMessage: "done.invoke.cancelWorkspace"
    assignCancellationError: "error.platform.cancelWorkspace"
    assignResources: "done.invoke.getResources"
    assignGetResourcesError: "error.platform.getResources"
    assignBuilds: "done.invoke.workspaceState.ready.timeline.gettingBuilds:invocation[0]"
    assignGetBuildsError: "error.platform.workspaceState.ready.timeline.gettingBuilds:invocation[0]"
    assignNewBuilds: "done.invoke.workspaceState.ready.timeline.loadedBuilds.loadingMoreBuilds:invocation[0]"
    assignLoadMoreBuildsError: "error.platform.workspaceState.ready.timeline.loadedBuilds.loadingMoreBuilds:invocation[0]"
    clearGetWorkspaceError: "GET_WORKSPACE"
    clearContext: "GET_WORKSPACE"
    clearRefreshTemplateError: "done.invoke.getWorkspace" | "UPDATE"
    clearGetPermissionsError: "done.invoke.refreshTemplate"
    clearGetBuildsError: "done.invoke.checkPermissions" | "REFRESH_TIMELINE"
    clearGetResourcesError:
      | "done.invoke.checkPermissions"
      | "xstate.after(5000)#workspaceState.ready.pollingResources.waiting"
    clearRefreshWorkspaceError:
      | "done.invoke.checkPermissions"
      | "xstate.after(1000)#workspaceState.ready.pollingWorkspace.waiting"
    clearBuildError: "START" | "done.invoke.refreshTemplate" | "STOP" | "DELETE"
    clearCancellationMessage: "CANCEL"
    clearCancellationError: "CANCEL"
    clearLoadMoreBuildsError: "LOAD_MORE_BUILDS"
  }
  eventsCausingServices: {
    getWorkspace: "GET_WORKSPACE"
    getTemplate: "done.invoke.getWorkspace" | "UPDATE"
    checkPermissions: "done.invoke.refreshTemplate"
    refreshWorkspace:
      | "done.invoke.checkPermissions"
      | "xstate.after(1000)#workspaceState.ready.pollingWorkspace.waiting"
    startWorkspace: "START" | "done.invoke.refreshTemplate"
    stopWorkspace: "STOP"
    deleteWorkspace: "DELETE"
    cancelWorkspace: "CANCEL"
    getResources:
      | "done.invoke.checkPermissions"
      | "xstate.after(5000)#workspaceState.ready.pollingResources.waiting"
    getBuilds: "done.invoke.checkPermissions" | "REFRESH_TIMELINE"
    loadMoreBuilds: "LOAD_MORE_BUILDS"
  }
  eventsCausingGuards: {
    hasMoreBuilds: "LOAD_MORE_BUILDS"
  }
  eventsCausingDelays: {}
  matchesStates:
    | "idle"
    | "gettingWorkspace"
    | "refreshingTemplate"
    | "gettingPermissions"
    | "ready"
    | "ready.pollingWorkspace"
    | "ready.pollingWorkspace.refreshingWorkspace"
    | "ready.pollingWorkspace.waiting"
    | "ready.build"
    | "ready.build.idle"
    | "ready.build.askingDelete"
    | "ready.build.requestingStart"
    | "ready.build.requestingStop"
    | "ready.build.requestingDelete"
    | "ready.build.requestingCancel"
    | "ready.build.refreshingTemplate"
    | "ready.pollingResources"
    | "ready.pollingResources.gettingResources"
    | "ready.pollingResources.waiting"
    | "ready.timeline"
    | "ready.timeline.idle"
    | "ready.timeline.gettingBuilds"
    | "ready.timeline.loadedBuilds"
    | "ready.timeline.loadedBuilds.idle"
    | "ready.timeline.loadedBuilds.loadingMoreBuilds"
    | "error"
    | {
        ready?:
          | "pollingWorkspace"
          | "build"
          | "pollingResources"
          | "timeline"
          | {
              pollingWorkspace?: "refreshingWorkspace" | "waiting"
              build?:
                | "idle"
                | "askingDelete"
                | "requestingStart"
                | "requestingStop"
                | "requestingDelete"
                | "requestingCancel"
                | "refreshingTemplate"
              pollingResources?: "gettingResources" | "waiting"
              timeline?:
                | "idle"
                | "gettingBuilds"
                | "loadedBuilds"
                | { loadedBuilds?: "idle" | "loadingMoreBuilds" }
            }
      }
  tags: "loading"
}
