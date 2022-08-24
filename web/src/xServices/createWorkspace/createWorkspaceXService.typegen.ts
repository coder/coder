// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.createWorkspaceState.gettingTemplates:invocation[0]": {
      type: "done.invoke.createWorkspaceState.gettingTemplates:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.createWorkspaceState.gettingTemplates:invocation[0]": {
      type: "error.platform.createWorkspaceState.gettingTemplates:invocation[0]"
      data: unknown
    }
    "done.invoke.createWorkspaceState.gettingTemplateSchema:invocation[0]": {
      type: "done.invoke.createWorkspaceState.gettingTemplateSchema:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.createWorkspaceState.gettingTemplateSchema:invocation[0]": {
      type: "error.platform.createWorkspaceState.gettingTemplateSchema:invocation[0]"
      data: unknown
    }
    "done.invoke.createWorkspaceState.creatingWorkspace:invocation[0]": {
      type: "done.invoke.createWorkspaceState.creatingWorkspace:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.createWorkspaceState.creatingWorkspace:invocation[0]": {
      type: "error.platform.createWorkspaceState.creatingWorkspace:invocation[0]"
      data: unknown
    }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    getTemplates: "done.invoke.createWorkspaceState.gettingTemplates:invocation[0]"
    getTemplateSchema: "done.invoke.createWorkspaceState.gettingTemplateSchema:invocation[0]"
    createWorkspace: "done.invoke.createWorkspaceState.creatingWorkspace:invocation[0]"
  }
  missingImplementations: {
    actions: "onCreateWorkspace"
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignTemplates: "done.invoke.createWorkspaceState.gettingTemplates:invocation[0]"
    assignSelectedTemplate: "done.invoke.createWorkspaceState.gettingTemplates:invocation[0]"
    assignGetTemplatesError: "error.platform.createWorkspaceState.gettingTemplates:invocation[0]"
    assignTemplateSchema: "done.invoke.createWorkspaceState.gettingTemplateSchema:invocation[0]"
    assignGetTemplateSchemaError: "error.platform.createWorkspaceState.gettingTemplateSchema:invocation[0]"
    assignCreateWorkspaceRequest: "CREATE_WORKSPACE"
    onCreateWorkspace: "done.invoke.createWorkspaceState.creatingWorkspace:invocation[0]"
    assignCreateWorkspaceError: "error.platform.createWorkspaceState.creatingWorkspace:invocation[0]"
    clearGetTemplatesError: "xstate.init"
    clearGetTemplateSchemaError: "done.invoke.createWorkspaceState.gettingTemplates:invocation[0]"
    clearCreateWorkspaceError: "CREATE_WORKSPACE"
  }
  eventsCausingServices: {
    getTemplates: "xstate.init"
    getTemplateSchema: "done.invoke.createWorkspaceState.gettingTemplates:invocation[0]"
    createWorkspace: "CREATE_WORKSPACE"
  }
  eventsCausingGuards: {
    areTemplatesEmpty: "done.invoke.createWorkspaceState.gettingTemplates:invocation[0]"
  }
  eventsCausingDelays: {}
  matchesStates:
    | "gettingTemplates"
    | "gettingTemplateSchema"
    | "fillingParams"
    | "creatingWorkspace"
    | "created"
    | "error"
  tags: never
}
