// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.templateVersion.initialInfo.template.loadingTemplate:invocation[0]": {
      type: "done.invoke.templateVersion.initialInfo.template.loadingTemplate:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.templateVersion.initialInfo.versions.loadingVersions:invocation[0]": {
      type: "done.invoke.templateVersion.initialInfo.versions.loadingVersions:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.templateVersion.loadingFiles:invocation[0]": {
      type: "done.invoke.templateVersion.loadingFiles:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.templateVersion.loadingFiles:invocation[0]": {
      type: "error.platform.templateVersion.loadingFiles:invocation[0]";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    loadFiles: "done.invoke.templateVersion.loadingFiles:invocation[0]";
    loadTemplate: "done.invoke.templateVersion.initialInfo.template.loadingTemplate:invocation[0]";
    loadVersions: "done.invoke.templateVersion.initialInfo.versions.loadingVersions:invocation[0]";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignError: "error.platform.templateVersion.loadingFiles:invocation[0]";
    assignFiles: "done.invoke.templateVersion.loadingFiles:invocation[0]";
    assignTemplate: "done.invoke.templateVersion.initialInfo.template.loadingTemplate:invocation[0]";
    assignVersions: "done.invoke.templateVersion.initialInfo.versions.loadingVersions:invocation[0]";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {
    loadFiles: "done.state.templateVersion.initialInfo";
    loadTemplate: "xstate.init";
    loadVersions: "xstate.init";
  };
  matchesStates:
    | "done"
    | "done.error"
    | "done.ok"
    | "initialInfo"
    | "initialInfo.template"
    | "initialInfo.template.loadingTemplate"
    | "initialInfo.template.success"
    | "initialInfo.versions"
    | "initialInfo.versions.loadingVersions"
    | "initialInfo.versions.success"
    | "loadingFiles"
    | {
        done?: "error" | "ok";
        initialInfo?:
          | "template"
          | "versions"
          | {
              template?: "loadingTemplate" | "success";
              versions?: "loadingVersions" | "success";
            };
      };
  tags: never;
}
