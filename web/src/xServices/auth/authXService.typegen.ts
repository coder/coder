// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "error.platform.signIn": { type: "error.platform.signIn"; data: unknown }
    "error.platform.signOut": { type: "error.platform.signOut"; data: unknown }
    "done.invoke.getMe": {
      type: "done.invoke.getMe"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]": {
      type: "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getMe": { type: "error.platform.getMe"; data: unknown }
    "done.invoke.checkPermissions": {
      type: "done.invoke.checkPermissions"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.checkPermissions": { type: "error.platform.checkPermissions"; data: unknown }
    "done.invoke.getMethods": {
      type: "done.invoke.getMethods"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getMethods": { type: "error.platform.getMethods"; data: unknown }
    "error.platform.authState.signedIn.profile.updatingProfile:invocation[0]": {
      type: "error.platform.authState.signedIn.profile.updatingProfile:invocation[0]"
      data: unknown
    }
    "done.invoke.authState.signedIn.ssh.gettingSSHKey:invocation[0]": {
      type: "done.invoke.authState.signedIn.ssh.gettingSSHKey:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.authState.signedIn.ssh.loaded.regeneratingSSHKey:invocation[0]": {
      type: "done.invoke.authState.signedIn.ssh.loaded.regeneratingSSHKey:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.authState.signedIn.ssh.gettingSSHKey:invocation[0]": {
      type: "error.platform.authState.signedIn.ssh.gettingSSHKey:invocation[0]"
      data: unknown
    }
    "error.platform.authState.signedIn.ssh.loaded.regeneratingSSHKey:invocation[0]": {
      type: "error.platform.authState.signedIn.ssh.loaded.regeneratingSSHKey:invocation[0]"
      data: unknown
    }
    "done.invoke.authState.signedIn.security.updatingSecurity:invocation[0]": {
      type: "done.invoke.authState.signedIn.security.updatingSecurity:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.authState.signedIn.security.updatingSecurity:invocation[0]": {
      type: "error.platform.authState.signedIn.security.updatingSecurity:invocation[0]"
      data: unknown
    }
    "done.invoke.signOut": {
      type: "done.invoke.signOut"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "xstate.init": { type: "xstate.init" }
    "done.invoke.signIn": {
      type: "done.invoke.signIn"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "done.invoke.authState.checkingFirstUser:invocation[0]": {
      type: "done.invoke.authState.checkingFirstUser:invocation[0]"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
  }
  invokeSrcNameMap: {
    signIn: "done.invoke.signIn"
    getMe: "done.invoke.getMe"
    checkPermissions: "done.invoke.checkPermissions"
    getMethods: "done.invoke.getMethods"
    updateProfile: "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]"
    getSSHKey: "done.invoke.authState.signedIn.ssh.gettingSSHKey:invocation[0]"
    regenerateSSHKey: "done.invoke.authState.signedIn.ssh.loaded.regeneratingSSHKey:invocation[0]"
    updateSecurity: "done.invoke.authState.signedIn.security.updatingSecurity:invocation[0]"
    signOut: "done.invoke.signOut"
    hasFirstUser: "done.invoke.authState.checkingFirstUser:invocation[0]"
  }
  missingImplementations: {
    actions: "redirectToSetupPage"
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignAuthError: "error.platform.signIn" | "error.platform.signOut"
    assignMe:
      | "done.invoke.getMe"
      | "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]"
    assignGetUserError: "error.platform.getMe"
    assignPermissions: "done.invoke.checkPermissions"
    assignGetPermissionsError: "error.platform.checkPermissions"
    assignMethods: "done.invoke.getMethods"
    clearGetMethodsError: "done.invoke.getMethods"
    assignGetMethodsError: "error.platform.getMethods"
    notifySuccessProfileUpdate: "done.invoke.authState.signedIn.profile.updatingProfile:invocation[0]"
    assignUpdateProfileError: "error.platform.authState.signedIn.profile.updatingProfile:invocation[0]"
    assignSSHKey:
      | "done.invoke.authState.signedIn.ssh.gettingSSHKey:invocation[0]"
      | "done.invoke.authState.signedIn.ssh.loaded.regeneratingSSHKey:invocation[0]"
    assignGetSSHKeyError: "error.platform.authState.signedIn.ssh.gettingSSHKey:invocation[0]"
    notifySuccessSSHKeyRegenerated: "done.invoke.authState.signedIn.ssh.loaded.regeneratingSSHKey:invocation[0]"
    assignRegenerateSSHKeyError: "error.platform.authState.signedIn.ssh.loaded.regeneratingSSHKey:invocation[0]"
    notifySuccessSecurityUpdate: "done.invoke.authState.signedIn.security.updatingSecurity:invocation[0]"
    assignUpdateSecurityError: "error.platform.authState.signedIn.security.updatingSecurity:invocation[0]"
    unassignMe: "done.invoke.signOut"
    clearAuthError: "done.invoke.signOut" | "SIGN_IN"
    clearGetUserError: "xstate.init" | "done.invoke.signIn"
    clearGetPermissionsError: "done.invoke.getMe"
    clearUpdateProfileError: "UPDATE_PROFILE"
    clearGetSSHKeyError: "GET_SSH_KEY"
    clearRegenerateSSHKeyError: "CONFIRM_REGENERATE_SSH_KEY"
    clearUpdateSecurityError: "UPDATE_SECURITY"
    redirectToSetupPage: "done.invoke.authState.checkingFirstUser:invocation[0]"
  }
  eventsCausingServices: {
    signIn: "SIGN_IN"
    getMe: "xstate.init" | "done.invoke.signIn"
    checkPermissions: "done.invoke.getMe"
    getMethods: "done.invoke.signOut" | "done.invoke.authState.checkingFirstUser:invocation[0]"
    updateProfile: "UPDATE_PROFILE"
    getSSHKey: "GET_SSH_KEY"
    regenerateSSHKey: "CONFIRM_REGENERATE_SSH_KEY"
    updateSecurity: "UPDATE_SECURITY"
    signOut: "SIGN_OUT"
    hasFirstUser: "error.platform.getMe"
  }
  eventsCausingGuards: {
    isTrue: "done.invoke.authState.checkingFirstUser:invocation[0]"
  }
  eventsCausingDelays: {}
  matchesStates:
    | "signedOut"
    | "signingIn"
    | "gettingUser"
    | "gettingPermissions"
    | "gettingMethods"
    | "signedIn"
    | "signedIn.profile"
    | "signedIn.profile.idle"
    | "signedIn.profile.idle.noError"
    | "signedIn.profile.idle.error"
    | "signedIn.profile.updatingProfile"
    | "signedIn.ssh"
    | "signedIn.ssh.idle"
    | "signedIn.ssh.gettingSSHKey"
    | "signedIn.ssh.loaded"
    | "signedIn.ssh.loaded.idle"
    | "signedIn.ssh.loaded.confirmSSHKeyRegenerate"
    | "signedIn.ssh.loaded.regeneratingSSHKey"
    | "signedIn.security"
    | "signedIn.security.idle"
    | "signedIn.security.idle.noError"
    | "signedIn.security.idle.error"
    | "signedIn.security.updatingSecurity"
    | "signingOut"
    | "checkingFirstUser"
    | "waitingForTheFirstUser"
    | {
        signedIn?:
          | "profile"
          | "ssh"
          | "security"
          | {
              profile?: "idle" | "updatingProfile" | { idle?: "noError" | "error" }
              ssh?:
                | "idle"
                | "gettingSSHKey"
                | "loaded"
                | { loaded?: "idle" | "confirmSSHKeyRegenerate" | "regeneratingSSHKey" }
              security?: "idle" | "updatingSecurity" | { idle?: "noError" | "error" }
            }
      }
  tags: "loading"
}
