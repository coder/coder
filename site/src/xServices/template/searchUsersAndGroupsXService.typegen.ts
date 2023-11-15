// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.searchUsersAndGroups.searching:invocation[0]": {
      type: "done.invoke.searchUsersAndGroups.searching:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    search: "done.invoke.searchUsersAndGroups.searching:invocation[0]";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignSearchResults: "done.invoke.searchUsersAndGroups.searching:invocation[0]";
    clearResults: "CLEAR_RESULTS";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {
    queryHasMinLength: "SEARCH";
  };
  eventsCausingServices: {
    search: "SEARCH";
  };
  matchesStates: "idle" | "searching";
  tags: never;
}
