// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.templateSettings.loading:invocation[0]": {
      type: "done.invoke.templateSettings.loading:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.templateSettings.loading:invocation[0]": {
      type: "error.platform.templateSettings.loading:invocation[0]"
      data: unknown
    }
    "error.platform.templateSettings.saving:invocation[0]": {
      type: "error.platform.templateSettings.saving:invocation[0]"
      data: unknown
    }
    "done.invoke.templateSettings.saving:invocation[0]": {
      type: "done.invoke.templateSettings.saving:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    getTemplateSettings: "done.invoke.templateSettings.loading:invocation[0]"
    saveTemplateSettings: "done.invoke.templateSettings.saving:invocation[0]"
  }
  missingImplementations: {
    actions: "onSave"
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignTemplateSettings: "done.invoke.templateSettings.loading:invocation[0]"
    assignGetTemplateError: "error.platform.templateSettings.loading:invocation[0]"
    assignSaveTemplateSettingsError: "error.platform.templateSettings.saving:invocation[0]"
    onSave: "done.invoke.templateSettings.saving:invocation[0]"
  }
  eventsCausingServices: {
    getTemplateSettings: "xstate.init"
    saveTemplateSettings: "SAVE"
  }
  eventsCausingGuards: {}
  eventsCausingDelays: {}
  matchesStates: "loading" | "editing" | "saving" | "saved" | "error"
  tags: "submitting"
}
