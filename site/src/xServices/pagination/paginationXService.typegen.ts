// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {};
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignNextPage: "NEXT_PAGE";
    assignPage: "GO_TO_PAGE";
    assignPreviousPage: "PREVIOUS_PAGE";
    resetPage: "RESET_PAGE";
    sendUpdatePage: "GO_TO_PAGE" | "NEXT_PAGE" | "PREVIOUS_PAGE" | "RESET_PAGE";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {};
  matchesStates: "ready";
  tags: never;
}
