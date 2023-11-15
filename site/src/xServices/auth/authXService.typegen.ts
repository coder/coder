// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.authState.loadingInitialAuthData:invocation[0]": {
      type: "done.invoke.authState.loadingInitialAuthData:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]": {
      type: "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.signIn": {
      type: "done.invoke.signIn";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.signOut": {
      type: "done.invoke.signOut";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.authState.loadingInitialAuthData:invocation[0]": {
      type: "error.platform.authState.loadingInitialAuthData:invocation[0]";
      data: unknown;
    };
    "error.platform.authState.signedIn.profile.updatingProfile:invocation[0]": {
      type: "error.platform.authState.signedIn.profile.updatingProfile:invocation[0]";
      data: unknown;
    };
    "error.platform.signIn": { type: "error.platform.signIn"; data: unknown };
    "error.platform.signOut": { type: "error.platform.signOut"; data: unknown };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    loadInitialAuthData: "done.invoke.authState.loadingInitialAuthData:invocation[0]";
    signIn: "done.invoke.signIn";
    signOut: "done.invoke.signOut";
    updateProfile: "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignData:
      | "done.invoke.authState.loadingInitialAuthData:invocation[0]"
      | "done.invoke.signIn";
    assignError:
      | "error.platform.authState.loadingInitialAuthData:invocation[0]"
      | "error.platform.signIn"
      | "error.platform.signOut";
    assignUpdateProfileError: "error.platform.authState.signedIn.profile.updatingProfile:invocation[0]";
    clearData: "done.invoke.signOut";
    clearError:
      | "SIGN_IN"
      | "done.invoke.authState.loadingInitialAuthData:invocation[0]"
      | "done.invoke.signOut";
    clearUpdateProfileError: "UPDATE_PROFILE";
    notifySuccessProfileUpdate: "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]";
    redirect: "done.invoke.signOut";
    updateUser: "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {
    hasRedirectUrl: "done.invoke.signOut";
    isAuthenticated: "done.invoke.authState.loadingInitialAuthData:invocation[0]";
    needSetup: "done.invoke.authState.loadingInitialAuthData:invocation[0]";
  };
  eventsCausingServices: {
    loadInitialAuthData: "xstate.init";
    signIn: "SIGN_IN";
    signOut: "SIGN_OUT";
    updateProfile: "UPDATE_PROFILE";
  };
  matchesStates:
    | "configuringTheFirstUser"
    | "loadingInitialAuthData"
    | "signedIn"
    | "signedIn.profile"
    | "signedIn.profile.idle"
    | "signedIn.profile.idle.error"
    | "signedIn.profile.idle.noError"
    | "signedIn.profile.updatingProfile"
    | "signedOut"
    | "signingIn"
    | "signingOut"
    | {
        signedIn?:
          | "profile"
          | {
              profile?:
                | "idle"
                | "updatingProfile"
                | { idle?: "error" | "noError" };
            };
      };
  tags: never;
}
