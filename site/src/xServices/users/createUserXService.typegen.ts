// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.createUser": {
      type: "done.invoke.createUser";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.createUser": {
      type: "error.platform.createUser";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    createUser: "done.invoke.createUser";
  };
  missingImplementations: {
    actions: "redirectToUsersPage";
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignError: "error.platform.createUser";
    clearError: "CANCEL_CREATE_USER" | "CREATE";
    displayCreateUserSuccess: "done.invoke.createUser";
    redirectToUsersPage: "done.invoke.createUser";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {
    createUser: "CREATE";
  };
  matchesStates: "creatingUser" | "idle";
  tags: "loading";
}
