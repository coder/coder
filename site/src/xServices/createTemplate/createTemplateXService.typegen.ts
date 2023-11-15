// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "": { type: "" };
    "done.invoke.createTemplate.copyingTemplateData:invocation[0]": {
      type: "done.invoke.createTemplate.copyingTemplateData:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createTemplate.creating.checkingParametersAndVariables:invocation[0]": {
      type: "done.invoke.createTemplate.creating.checkingParametersAndVariables:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createTemplate.creating.creatingFirstVersion:invocation[0]": {
      type: "done.invoke.createTemplate.creating.creatingFirstVersion:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createTemplate.creating.creatingTemplate:invocation[0]": {
      type: "done.invoke.createTemplate.creating.creatingTemplate:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createTemplate.creating.creatingVersionWithParametersAndVariables:invocation[0]": {
      type: "done.invoke.createTemplate.creating.creatingVersionWithParametersAndVariables:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createTemplate.creating.loadingVersionLogs:invocation[0]": {
      type: "done.invoke.createTemplate.creating.loadingVersionLogs:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]": {
      type: "done.invoke.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createTemplate.loadingStarterTemplate:invocation[0]": {
      type: "done.invoke.createTemplate.loadingStarterTemplate:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createTemplate.uploading:invocation[0]": {
      type: "done.invoke.createTemplate.uploading:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.createTemplate.copyingTemplateData:invocation[0]": {
      type: "error.platform.createTemplate.copyingTemplateData:invocation[0]";
      data: unknown;
    };
    "error.platform.createTemplate.creating.checkingParametersAndVariables:invocation[0]": {
      type: "error.platform.createTemplate.creating.checkingParametersAndVariables:invocation[0]";
      data: unknown;
    };
    "error.platform.createTemplate.creating.creatingFirstVersion:invocation[0]": {
      type: "error.platform.createTemplate.creating.creatingFirstVersion:invocation[0]";
      data: unknown;
    };
    "error.platform.createTemplate.creating.creatingTemplate:invocation[0]": {
      type: "error.platform.createTemplate.creating.creatingTemplate:invocation[0]";
      data: unknown;
    };
    "error.platform.createTemplate.creating.creatingVersionWithParametersAndVariables:invocation[0]": {
      type: "error.platform.createTemplate.creating.creatingVersionWithParametersAndVariables:invocation[0]";
      data: unknown;
    };
    "error.platform.createTemplate.creating.loadingVersionLogs:invocation[0]": {
      type: "error.platform.createTemplate.creating.loadingVersionLogs:invocation[0]";
      data: unknown;
    };
    "error.platform.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]": {
      type: "error.platform.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]";
      data: unknown;
    };
    "error.platform.createTemplate.loadingStarterTemplate:invocation[0]": {
      type: "error.platform.createTemplate.loadingStarterTemplate:invocation[0]";
      data: unknown;
    };
    "error.platform.createTemplate.uploading:invocation[0]": {
      type: "error.platform.createTemplate.uploading:invocation[0]";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    checkParametersAndVariables: "done.invoke.createTemplate.creating.checkingParametersAndVariables:invocation[0]";
    copyTemplateData: "done.invoke.createTemplate.copyingTemplateData:invocation[0]";
    createFirstVersion: "done.invoke.createTemplate.creating.creatingFirstVersion:invocation[0]";
    createTemplate: "done.invoke.createTemplate.creating.creatingTemplate:invocation[0]";
    createVersionWithParametersAndVariables: "done.invoke.createTemplate.creating.creatingVersionWithParametersAndVariables:invocation[0]";
    loadStarterTemplate: "done.invoke.createTemplate.loadingStarterTemplate:invocation[0]";
    loadVersionLogs: "done.invoke.createTemplate.creating.loadingVersionLogs:invocation[0]";
    uploadFile: "done.invoke.createTemplate.uploading:invocation[0]";
    waitForJobToBeCompleted: "done.invoke.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]";
  };
  missingImplementations: {
    actions: "onCreate";
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignCopiedTemplateData: "done.invoke.createTemplate.copyingTemplateData:invocation[0]";
    assignError:
      | "error.platform.createTemplate.copyingTemplateData:invocation[0]"
      | "error.platform.createTemplate.creating.checkingParametersAndVariables:invocation[0]"
      | "error.platform.createTemplate.creating.creatingFirstVersion:invocation[0]"
      | "error.platform.createTemplate.creating.creatingTemplate:invocation[0]"
      | "error.platform.createTemplate.creating.creatingVersionWithParametersAndVariables:invocation[0]"
      | "error.platform.createTemplate.creating.loadingVersionLogs:invocation[0]"
      | "error.platform.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]"
      | "error.platform.createTemplate.loadingStarterTemplate:invocation[0]";
    assignFile: "UPLOAD_FILE";
    assignJobError: "done.invoke.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]";
    assignJobLogs: "done.invoke.createTemplate.creating.loadingVersionLogs:invocation[0]";
    assignParametersAndVariables: "done.invoke.createTemplate.creating.checkingParametersAndVariables:invocation[0]";
    assignStarterTemplate: "done.invoke.createTemplate.loadingStarterTemplate:invocation[0]";
    assignTemplateData: "CREATE";
    assignUploadResponse: "done.invoke.createTemplate.uploading:invocation[0]";
    assignVersion:
      | "done.invoke.createTemplate.creating.creatingFirstVersion:invocation[0]"
      | "done.invoke.createTemplate.creating.creatingVersionWithParametersAndVariables:invocation[0]"
      | "done.invoke.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]";
    displayUploadError: "error.platform.createTemplate.uploading:invocation[0]";
    onCreate: "done.invoke.createTemplate.creating.creatingTemplate:invocation[0]";
    removeFile:
      | "REMOVE_FILE"
      | "error.platform.createTemplate.uploading:invocation[0]";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {
    hasFailed: "done.invoke.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]";
    hasFile: "REMOVE_FILE";
    hasNoParametersOrVariables: "done.invoke.createTemplate.creating.checkingParametersAndVariables:invocation[0]";
    hasParametersOrVariables: "done.invoke.createTemplate.copyingTemplateData:invocation[0]";
    isExampleProvided: "";
    isNotUsingExample: "UPLOAD_FILE";
    isTemplateIdToCopyProvided: "";
  };
  eventsCausingServices: {
    checkParametersAndVariables: "done.invoke.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]";
    copyTemplateData: "";
    createFirstVersion:
      | "CREATE"
      | "done.invoke.createTemplate.copyingTemplateData:invocation[0]";
    createTemplate: "done.invoke.createTemplate.creating.checkingParametersAndVariables:invocation[0]";
    createVersionWithParametersAndVariables: "CREATE";
    loadStarterTemplate: "";
    loadVersionLogs: "done.invoke.createTemplate.creating.waitingForJobToBeCompleted:invocation[0]";
    uploadFile: "UPLOAD_FILE";
    waitForJobToBeCompleted:
      | "done.invoke.createTemplate.creating.creatingFirstVersion:invocation[0]"
      | "done.invoke.createTemplate.creating.creatingVersionWithParametersAndVariables:invocation[0]";
  };
  matchesStates:
    | "copyingTemplateData"
    | "creating"
    | "creating.checkingParametersAndVariables"
    | "creating.created"
    | "creating.creatingFirstVersion"
    | "creating.creatingTemplate"
    | "creating.creatingVersionWithParametersAndVariables"
    | "creating.loadingVersionLogs"
    | "creating.promptParametersAndVariables"
    | "creating.waitingForJobToBeCompleted"
    | "idle"
    | "loadingStarterTemplate"
    | "starting"
    | "uploading"
    | {
        creating?:
          | "checkingParametersAndVariables"
          | "created"
          | "creatingFirstVersion"
          | "creatingTemplate"
          | "creatingVersionWithParametersAndVariables"
          | "loadingVersionLogs"
          | "promptParametersAndVariables"
          | "waitingForJobToBeCompleted";
      };
  tags: "loading" | "submitting";
}
