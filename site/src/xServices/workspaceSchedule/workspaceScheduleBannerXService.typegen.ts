// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.decreaseDeadline": {
      type: "done.invoke.decreaseDeadline";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.increaseDeadline": {
      type: "done.invoke.increaseDeadline";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.decreaseDeadline": {
      type: "error.platform.decreaseDeadline";
      data: unknown;
    };
    "error.platform.increaseDeadline": {
      type: "error.platform.increaseDeadline";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    decreaseDeadline: "done.invoke.decreaseDeadline";
    increaseDeadline: "done.invoke.increaseDeadline";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignWorkspace: "REFRESH_WORKSPACE";
    displayFailureMessage:
      | "error.platform.decreaseDeadline"
      | "error.platform.increaseDeadline";
    displaySuccessMessage:
      | "done.invoke.decreaseDeadline"
      | "done.invoke.increaseDeadline";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {
    decreaseDeadline: "DECREASE_DEADLINE";
    increaseDeadline: "INCREASE_DEADLINE";
  };
  matchesStates: "decreasingDeadline" | "idle" | "increasingDeadline";
  tags: "loading";
}
