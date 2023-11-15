// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.activateWorkspace": {
      type: "done.invoke.activateWorkspace";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.cancelWorkspace": {
      type: "done.invoke.cancelWorkspace";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.deleteWorkspace": {
      type: "done.invoke.deleteWorkspace";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.listening": {
      type: "done.invoke.listening";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.loadInitialWorkspaceData": {
      type: "done.invoke.loadInitialWorkspaceData";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.scheduleBannerMachine": {
      type: "done.invoke.scheduleBannerMachine";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.startWorkspace": {
      type: "done.invoke.startWorkspace";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.stopWorkspace": {
      type: "done.invoke.stopWorkspace";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.workspaceState.ready.build.requestingChangeVersion:invocation[0]": {
      type: "done.invoke.workspaceState.ready.build.requestingChangeVersion:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.workspaceState.ready.build.requestingUpdate:invocation[0]": {
      type: "done.invoke.workspaceState.ready.build.requestingUpdate:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.workspaceState.ready.sshConfig.gettingSshConfig:invocation[0]": {
      type: "done.invoke.workspaceState.ready.sshConfig.gettingSshConfig:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.activateWorkspace": {
      type: "error.platform.activateWorkspace";
      data: unknown;
    };
    "error.platform.cancelWorkspace": {
      type: "error.platform.cancelWorkspace";
      data: unknown;
    };
    "error.platform.deleteWorkspace": {
      type: "error.platform.deleteWorkspace";
      data: unknown;
    };
    "error.platform.listening": {
      type: "error.platform.listening";
      data: unknown;
    };
    "error.platform.loadInitialWorkspaceData": {
      type: "error.platform.loadInitialWorkspaceData";
      data: unknown;
    };
    "error.platform.scheduleBannerMachine": {
      type: "error.platform.scheduleBannerMachine";
      data: unknown;
    };
    "error.platform.startWorkspace": {
      type: "error.platform.startWorkspace";
      data: unknown;
    };
    "error.platform.stopWorkspace": {
      type: "error.platform.stopWorkspace";
      data: unknown;
    };
    "error.platform.workspaceState.ready.build.requestingChangeVersion:invocation[0]": {
      type: "error.platform.workspaceState.ready.build.requestingChangeVersion:invocation[0]";
      data: unknown;
    };
    "error.platform.workspaceState.ready.build.requestingUpdate:invocation[0]": {
      type: "error.platform.workspaceState.ready.build.requestingUpdate:invocation[0]";
      data: unknown;
    };
    "error.platform.workspaceState.ready.sshConfig.gettingSshConfig:invocation[0]": {
      type: "error.platform.workspaceState.ready.sshConfig.gettingSshConfig:invocation[0]";
      data: unknown;
    };
    "xstate.after(2000)#workspaceState.ready.listening.error": {
      type: "xstate.after(2000)#workspaceState.ready.listening.error";
    };
    "xstate.init": { type: "xstate.init" };
    "xstate.stop": { type: "xstate.stop" };
  };
  invokeSrcNameMap: {
    activateWorkspace: "done.invoke.activateWorkspace";
    cancelWorkspace: "done.invoke.cancelWorkspace";
    changeWorkspaceVersion: "done.invoke.workspaceState.ready.build.requestingChangeVersion:invocation[0]";
    deleteWorkspace: "done.invoke.deleteWorkspace";
    getSSHPrefix: "done.invoke.workspaceState.ready.sshConfig.gettingSshConfig:invocation[0]";
    listening: "done.invoke.listening";
    loadInitialWorkspaceData: "done.invoke.loadInitialWorkspaceData";
    scheduleBannerMachine: "done.invoke.scheduleBannerMachine";
    startWorkspace: "done.invoke.startWorkspace";
    stopWorkspace: "done.invoke.stopWorkspace";
    updateWorkspace: "done.invoke.workspaceState.ready.build.requestingUpdate:invocation[0]";
  };
  missingImplementations: {
    actions: "refreshBuilds";
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignBuild:
      | "done.invoke.deleteWorkspace"
      | "done.invoke.startWorkspace"
      | "done.invoke.stopWorkspace"
      | "done.invoke.workspaceState.ready.build.requestingChangeVersion:invocation[0]"
      | "done.invoke.workspaceState.ready.build.requestingUpdate:invocation[0]";
    assignBuildError:
      | "error.platform.deleteWorkspace"
      | "error.platform.startWorkspace"
      | "error.platform.stopWorkspace"
      | "error.platform.workspaceState.ready.build.requestingChangeVersion:invocation[0]"
      | "error.platform.workspaceState.ready.build.requestingUpdate:invocation[0]";
    assignCancellationError: "error.platform.cancelWorkspace";
    assignCancellationMessage: "done.invoke.cancelWorkspace";
    assignError: "error.platform.loadInitialWorkspaceData";
    assignInitialData: "done.invoke.loadInitialWorkspaceData";
    assignMissedParameters:
      | "error.platform.workspaceState.ready.build.requestingChangeVersion:invocation[0]"
      | "error.platform.workspaceState.ready.build.requestingUpdate:invocation[0]";
    assignSSHPrefix: "done.invoke.workspaceState.ready.sshConfig.gettingSshConfig:invocation[0]";
    assignTemplateVersionIdToChange: "CHANGE_VERSION";
    clearBuildError:
      | "ACTIVATE"
      | "CHANGE_VERSION"
      | "DELETE"
      | "RETRY_BUILD"
      | "START"
      | "STOP"
      | "UPDATE";
    clearCancellationError: "CANCEL";
    clearCancellationMessage: "CANCEL";
    clearContext: "xstate.init";
    clearTemplateVersionIdToChange: "done.invoke.workspaceState.ready.build.requestingChangeVersion:invocation[0]";
    closeEventSource: "EVENT_SOURCE_ERROR" | "xstate.stop";
    disableDebugMode:
      | "done.invoke.deleteWorkspace"
      | "done.invoke.startWorkspace"
      | "done.invoke.stopWorkspace";
    displayActivateError: "error.platform.activateWorkspace";
    displayCancellationMessage: "done.invoke.cancelWorkspace";
    displaySSHPrefixError: "error.platform.workspaceState.ready.sshConfig.gettingSshConfig:invocation[0]";
    enableDebugMode: "RETRY_BUILD";
    initializeEventSource:
      | "done.invoke.loadInitialWorkspaceData"
      | "xstate.after(2000)#workspaceState.ready.listening.error";
    logWatchWorkspaceWarning: "EVENT_SOURCE_ERROR";
    refreshBuilds: "REFRESH_TIMELINE";
    refreshWorkspace: "REFRESH_WORKSPACE";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {
    isChangingVersion: "UPDATE";
    isMissingBuildParameterError:
      | "error.platform.workspaceState.ready.build.requestingChangeVersion:invocation[0]"
      | "error.platform.workspaceState.ready.build.requestingUpdate:invocation[0]";
    lastBuildWasDeleting: "RETRY_BUILD";
    lastBuildWasStarting: "RETRY_BUILD";
    lastBuildWasStopping: "RETRY_BUILD";
  };
  eventsCausingServices: {
    activateWorkspace: "ACTIVATE";
    cancelWorkspace: "CANCEL";
    changeWorkspaceVersion: "CHANGE_VERSION" | "UPDATE";
    deleteWorkspace: "DELETE" | "RETRY_BUILD";
    getSSHPrefix: "done.invoke.loadInitialWorkspaceData";
    listening:
      | "done.invoke.loadInitialWorkspaceData"
      | "xstate.after(2000)#workspaceState.ready.listening.error";
    loadInitialWorkspaceData: "xstate.init";
    scheduleBannerMachine: "done.invoke.loadInitialWorkspaceData";
    startWorkspace: "RETRY_BUILD" | "START";
    stopWorkspace: "RETRY_BUILD" | "STOP";
    updateWorkspace: "UPDATE";
  };
  matchesStates:
    | "error"
    | "loadInitialData"
    | "ready"
    | "ready.build"
    | "ready.build.askingDelete"
    | "ready.build.askingForMissedBuildParameters"
    | "ready.build.idle"
    | "ready.build.requestingActivate"
    | "ready.build.requestingCancel"
    | "ready.build.requestingChangeVersion"
    | "ready.build.requestingDelete"
    | "ready.build.requestingStart"
    | "ready.build.requestingStop"
    | "ready.build.requestingUpdate"
    | "ready.listening"
    | "ready.listening.error"
    | "ready.listening.gettingEvents"
    | "ready.schedule"
    | "ready.sshConfig"
    | "ready.sshConfig.error"
    | "ready.sshConfig.gettingSshConfig"
    | "ready.sshConfig.success"
    | {
        ready?:
          | "build"
          | "listening"
          | "schedule"
          | "sshConfig"
          | {
              build?:
                | "askingDelete"
                | "askingForMissedBuildParameters"
                | "idle"
                | "requestingActivate"
                | "requestingCancel"
                | "requestingChangeVersion"
                | "requestingDelete"
                | "requestingStart"
                | "requestingStop"
                | "requestingUpdate";
              listening?: "error" | "gettingEvents";
              sshConfig?: "error" | "gettingSshConfig" | "success";
            };
      };
  tags: never;
}
