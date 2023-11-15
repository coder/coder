// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.checkPermissions": {
      type: "done.invoke.checkPermissions";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.getTemplate": {
      type: "done.invoke.getTemplate";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.submitSchedule": {
      type: "done.invoke.submitSchedule";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.checkPermissions": {
      type: "error.platform.checkPermissions";
      data: unknown;
    };
    "error.platform.getTemplate": {
      type: "error.platform.getTemplate";
      data: unknown;
    };
    "error.platform.submitSchedule": {
      type: "error.platform.submitSchedule";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    checkPermissions: "done.invoke.checkPermissions";
    getTemplate: "done.invoke.getTemplate";
    submitSchedule: "done.invoke.submitSchedule";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignAutostopChanged: "SUBMIT_SCHEDULE";
    assignGetPermissionsError: "error.platform.checkPermissions";
    assignGetTemplateError: "error.platform.getTemplate";
    assignPermissions: "done.invoke.checkPermissions";
    assignSubmissionError: "error.platform.submitSchedule";
    assignTemplate: "done.invoke.getTemplate";
    clearGetPermissionsError: "xstate.init";
    clearGetTemplateError: "done.invoke.checkPermissions";
    restartWorkspace: "RESTART_WORKSPACE";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {
    autostopChanged: "done.invoke.submitSchedule";
  };
  eventsCausingServices: {
    checkPermissions: "xstate.init";
    getTemplate: "done.invoke.checkPermissions";
    submitSchedule: "SUBMIT_SCHEDULE";
  };
  matchesStates:
    | "done"
    | "error"
    | "gettingPermissions"
    | "gettingTemplate"
    | "presentForm"
    | "showingRestartDialog"
    | "submittingSchedule";
  tags: "loading";
}
