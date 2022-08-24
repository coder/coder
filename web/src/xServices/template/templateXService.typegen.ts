// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.(machine).gettingTemplate:invocation[0]": {
      type: "done.invoke.(machine).gettingTemplate:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.(machine).initialInfo.activeTemplateVersion.gettingActiveTemplateVersion:invocation[0]": {
      type: "done.invoke.(machine).initialInfo.activeTemplateVersion.gettingActiveTemplateVersion:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.(machine).initialInfo.templateResources.gettingTemplateResources:invocation[0]": {
      type: "done.invoke.(machine).initialInfo.templateResources.gettingTemplateResources:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.(machine).initialInfo.templateVersions.gettingTemplateVersions:invocation[0]": {
      type: "done.invoke.(machine).initialInfo.templateVersions.gettingTemplateVersions:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    getTemplate: "done.invoke.(machine).gettingTemplate:invocation[0]"
    getActiveTemplateVersion: "done.invoke.(machine).initialInfo.activeTemplateVersion.gettingActiveTemplateVersion:invocation[0]"
    getTemplateResources: "done.invoke.(machine).initialInfo.templateResources.gettingTemplateResources:invocation[0]"
    getTemplateVersions: "done.invoke.(machine).initialInfo.templateVersions.gettingTemplateVersions:invocation[0]"
  }
  missingImplementations: {
    actions: never
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignTemplate: "done.invoke.(machine).gettingTemplate:invocation[0]"
    assignActiveTemplateVersion: "done.invoke.(machine).initialInfo.activeTemplateVersion.gettingActiveTemplateVersion:invocation[0]"
    assignTemplateResources: "done.invoke.(machine).initialInfo.templateResources.gettingTemplateResources:invocation[0]"
    assignTemplateVersions: "done.invoke.(machine).initialInfo.templateVersions.gettingTemplateVersions:invocation[0]"
  }
  eventsCausingServices: {
    getTemplate: "xstate.init"
    getActiveTemplateVersion: "done.invoke.(machine).gettingTemplate:invocation[0]"
    getTemplateResources: "done.invoke.(machine).gettingTemplate:invocation[0]"
    getTemplateVersions: "done.invoke.(machine).gettingTemplate:invocation[0]"
  }
  eventsCausingGuards: {}
  eventsCausingDelays: {}
  matchesStates:
    | "gettingTemplate"
    | "initialInfo"
    | "initialInfo.activeTemplateVersion"
    | "initialInfo.activeTemplateVersion.gettingActiveTemplateVersion"
    | "initialInfo.activeTemplateVersion.success"
    | "initialInfo.templateResources"
    | "initialInfo.templateResources.gettingTemplateResources"
    | "initialInfo.templateResources.success"
    | "initialInfo.templateVersions"
    | "initialInfo.templateVersions.gettingTemplateVersions"
    | "initialInfo.templateVersions.success"
    | "loaded"
    | {
        initialInfo?:
          | "activeTemplateVersion"
          | "templateResources"
          | "templateVersions"
          | {
              activeTemplateVersion?: "gettingActiveTemplateVersion" | "success"
              templateResources?: "gettingTemplateResources" | "success"
              templateVersions?: "gettingTemplateVersions" | "success"
            }
      }
  tags: never
}
