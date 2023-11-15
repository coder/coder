// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "": { type: "" };
    "done.invoke.createWorkspaceState.autoCreating:invocation[0]": {
      type: "done.invoke.createWorkspaceState.autoCreating:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createWorkspaceState.creatingWorkspace:invocation[0]": {
      type: "done.invoke.createWorkspaceState.creatingWorkspace:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.createWorkspaceState.loadingFormData:invocation[0]": {
      type: "done.invoke.createWorkspaceState.loadingFormData:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.createWorkspaceState.autoCreating:invocation[0]": {
      type: "error.platform.createWorkspaceState.autoCreating:invocation[0]";
      data: unknown;
    };
    "error.platform.createWorkspaceState.creatingWorkspace:invocation[0]": {
      type: "error.platform.createWorkspaceState.creatingWorkspace:invocation[0]";
      data: unknown;
    };
    "error.platform.createWorkspaceState.loadingFormData:invocation[0]": {
      type: "error.platform.createWorkspaceState.loadingFormData:invocation[0]";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    autoCreateWorkspace: "done.invoke.createWorkspaceState.autoCreating:invocation[0]";
    createWorkspace: "done.invoke.createWorkspaceState.creatingWorkspace:invocation[0]";
    loadFormData: "done.invoke.createWorkspaceState.loadingFormData:invocation[0]";
  };
  missingImplementations: {
    actions: "onCreateWorkspace";
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignError:
      | "error.platform.createWorkspaceState.autoCreating:invocation[0]"
      | "error.platform.createWorkspaceState.creatingWorkspace:invocation[0]"
      | "error.platform.createWorkspaceState.loadingFormData:invocation[0]";
    assignFormData: "done.invoke.createWorkspaceState.loadingFormData:invocation[0]";
    clearError: "CREATE_WORKSPACE";
    onCreateWorkspace:
      | "done.invoke.createWorkspaceState.autoCreating:invocation[0]"
      | "done.invoke.createWorkspaceState.creatingWorkspace:invocation[0]";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {
    autoCreateWorkspace: "";
    createWorkspace: "CREATE_WORKSPACE";
    loadFormData:
      | ""
      | "error.platform.createWorkspaceState.autoCreating:invocation[0]";
  };
  matchesStates:
    | "autoCreating"
    | "checkingMode"
    | "created"
    | "creatingWorkspace"
    | "idle"
    | "loadError"
    | "loadingFormData";
  tags: never;
}
