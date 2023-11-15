// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.cancelBuild": {
      type: "done.invoke.cancelBuild";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createBuild": {
      type: "done.invoke.createBuild";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.fetchVersion": {
      type: "done.invoke.fetchVersion";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.getResources": {
      type: "done.invoke.getResources";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.publishingVersion": {
      type: "done.invoke.publishingVersion";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.templateVersionEditor.promptVariables.loadingMissingVariables:invocation[0]": {
      type: "done.invoke.templateVersionEditor.promptVariables.loadingMissingVariables:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.uploadTar": {
      type: "done.invoke.uploadTar";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.watchBuildLogs": {
      type: "done.invoke.watchBuildLogs";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.cancelBuild": {
      type: "error.platform.cancelBuild";
      data: unknown;
    };
    "error.platform.createBuild": {
      type: "error.platform.createBuild";
      data: unknown;
    };
    "error.platform.fetchVersion": {
      type: "error.platform.fetchVersion";
      data: unknown;
    };
    "error.platform.getResources": {
      type: "error.platform.getResources";
      data: unknown;
    };
    "error.platform.publishingVersion": {
      type: "error.platform.publishingVersion";
      data: unknown;
    };
    "error.platform.uploadTar": {
      type: "error.platform.uploadTar";
      data: unknown;
    };
    "error.platform.watchBuildLogs": {
      type: "error.platform.watchBuildLogs";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    cancelBuild: "done.invoke.cancelBuild";
    createBuild: "done.invoke.createBuild";
    fetchVersion: "done.invoke.fetchVersion";
    getResources: "done.invoke.getResources";
    loadMissingVariables: "done.invoke.templateVersionEditor.promptVariables.loadingMissingVariables:invocation[0]";
    publishingVersion: "done.invoke.publishingVersion";
    uploadTar: "done.invoke.uploadTar";
    watchBuildLogs: "done.invoke.watchBuildLogs";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    addBuildLog: "ADD_BUILD_LOG";
    assignBuild: "done.invoke.createBuild" | "done.invoke.fetchVersion";
    assignCreateBuild: "CREATE_VERSION";
    assignLastSuccessfulPublishedVersion: "done.invoke.publishingVersion";
    assignMissingVariableValues: "SET_MISSING_VARIABLE_VALUES";
    assignMissingVariables: "done.invoke.templateVersionEditor.promptVariables.loadingMissingVariables:invocation[0]";
    assignPublishingError: "error.platform.publishingVersion";
    assignResources: "done.invoke.getResources";
    assignTarReader: "INITIALIZE";
    assignUploadResponse: "done.invoke.uploadTar";
    clearLastSuccessfulPublishedVersion: "CONFIRM_PUBLISH";
    clearPublishingError: "CONFIRM_PUBLISH";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {
    jobFailedWithMissingVariables: "done.invoke.fetchVersion";
  };
  eventsCausingServices: {
    cancelBuild: "CANCEL_VERSION" | "CREATE_VERSION";
    createBuild: "SET_MISSING_VARIABLE_VALUES" | "done.invoke.uploadTar";
    fetchVersion: "BUILD_DONE";
    getResources: "done.invoke.fetchVersion";
    loadMissingVariables: "done.invoke.fetchVersion";
    publishingVersion: "CONFIRM_PUBLISH";
    uploadTar: "CREATE_VERSION" | "done.invoke.cancelBuild";
    watchBuildLogs: "done.invoke.createBuild";
  };
  matchesStates:
    | "askPublishParameters"
    | "cancelingInProgressBuild"
    | "creatingBuild"
    | "fetchResources"
    | "fetchingVersion"
    | "idle"
    | "initializing"
    | "promptVariables"
    | "promptVariables.idle"
    | "promptVariables.loadingMissingVariables"
    | "publishingVersion"
    | "uploadTar"
    | "watchingBuildLogs"
    | { promptVariables?: "idle" | "loadingMissingVariables" };
  tags: "loading";
}
