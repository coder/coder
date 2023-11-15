// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "": { type: "" };
    "done.invoke.getUpdateCheck": {
      type: "done.invoke.getUpdateCheck";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.getUpdateCheck": {
      type: "error.platform.getUpdateCheck";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    getUpdateCheck: "done.invoke.getUpdateCheck";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignError: "error.platform.getUpdateCheck";
    assignUpdateCheck: "done.invoke.getUpdateCheck";
    setDismissedVersion: "DISMISS";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {
    canViewUpdateCheck: "";
    shouldShowUpdateCheck: "done.invoke.getUpdateCheck";
  };
  eventsCausingServices: {
    getUpdateCheck: "";
  };
  matchesStates:
    | "checkingPermissions"
    | "dismissed"
    | "fetchingUpdateCheck"
    | "show";
  tags: never;
}
