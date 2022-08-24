// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true
  internalEvents: {
    "done.invoke.getUsers": {
      type: "done.invoke.getUsers"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.getUsers": { type: "error.platform.getUsers"; data: unknown }
    "done.invoke.createUser": {
      type: "done.invoke.createUser"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.createUser": { type: "error.platform.createUser"; data: unknown }
    "done.invoke.suspendUser": {
      type: "done.invoke.suspendUser"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.suspendUser": { type: "error.platform.suspendUser"; data: unknown }
    "done.invoke.activateUser": {
      type: "done.invoke.activateUser"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.activateUser": { type: "error.platform.activateUser"; data: unknown }
    "done.invoke.resetUserPassword": {
      type: "done.invoke.resetUserPassword"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.resetUserPassword": { type: "error.platform.resetUserPassword"; data: unknown }
    "done.invoke.updateUserRoles": {
      type: "done.invoke.updateUserRoles"
      data: unknown
      __tip: "See the XState TS docs to learn how to strongly type this."
    }
    "error.platform.updateUserRoles": { type: "error.platform.updateUserRoles"; data: unknown }
    "xstate.init": { type: "xstate.init" }
  }
  invokeSrcNameMap: {
    getUsers: "done.invoke.getUsers"
    createUser: "done.invoke.createUser"
    suspendUser: "done.invoke.suspendUser"
    activateUser: "done.invoke.activateUser"
    resetUserPassword: "done.invoke.resetUserPassword"
    updateUserRoles: "done.invoke.updateUserRoles"
  }
  missingImplementations: {
    actions: "redirectToUsersPage"
    services: never
    guards: never
    delays: never
  }
  eventsCausingActions: {
    assignFilter: "GET_USERS"
    clearCreateUserError: "CANCEL_CREATE_USER" | "CREATE"
    assignUserIdToSuspend: "SUSPEND_USER"
    assignUserIdToActivate: "ACTIVATE_USER"
    assignUserIdToResetPassword: "RESET_USER_PASSWORD"
    generateRandomPassword: "RESET_USER_PASSWORD"
    assignUserIdToUpdateRoles: "UPDATE_USER_ROLES"
    assignUsers: "done.invoke.getUsers"
    clearUsers: "error.platform.getUsers"
    assignGetUsersError: "error.platform.getUsers"
    displayGetUsersErrorMessage: "error.platform.getUsers"
    displayCreateUserSuccess: "done.invoke.createUser"
    redirectToUsersPage: "done.invoke.createUser"
    assignCreateUserFormErrors: "error.platform.createUser"
    assignCreateUserError: "error.platform.createUser"
    displaySuspendSuccess: "done.invoke.suspendUser"
    assignSuspendUserError: "error.platform.suspendUser"
    displaySuspendedErrorMessage: "error.platform.suspendUser"
    displayActivateSuccess: "done.invoke.activateUser"
    assignActivateUserError: "error.platform.activateUser"
    displayActivatedErrorMessage: "error.platform.activateUser"
    displayResetPasswordSuccess: "done.invoke.resetUserPassword"
    assignResetUserPasswordError: "error.platform.resetUserPassword"
    displayResetPasswordErrorMessage: "error.platform.resetUserPassword"
    updateUserRolesInTheList: "done.invoke.updateUserRoles"
    assignUpdateRolesError: "error.platform.updateUserRoles"
    displayUpdateRolesErrorMessage: "error.platform.updateUserRoles"
    clearGetUsersError: "GET_USERS" | "done.invoke.suspendUser" | "done.invoke.activateUser"
    clearSuspendUserError: "CONFIRM_USER_SUSPENSION"
    clearActivateUserError: "CONFIRM_USER_ACTIVATION"
    clearResetUserPasswordError: "CONFIRM_USER_PASSWORD_RESET"
    clearUpdateUserRolesError: "UPDATE_USER_ROLES"
  }
  eventsCausingServices: {
    getUsers: "GET_USERS" | "done.invoke.suspendUser" | "done.invoke.activateUser"
    createUser: "CREATE"
    suspendUser: "CONFIRM_USER_SUSPENSION"
    activateUser: "CONFIRM_USER_ACTIVATION"
    resetUserPassword: "CONFIRM_USER_PASSWORD_RESET"
    updateUserRoles: "UPDATE_USER_ROLES"
  }
  eventsCausingGuards: {
    hasFieldErrors: "error.platform.createUser"
  }
  eventsCausingDelays: {}
  matchesStates:
    | "idle"
    | "gettingUsers"
    | "creatingUser"
    | "confirmUserSuspension"
    | "confirmUserActivation"
    | "suspendingUser"
    | "activatingUser"
    | "confirmUserPasswordReset"
    | "resettingUserPassword"
    | "updatingUserRoles"
    | "error"
  tags: "loading"
}
