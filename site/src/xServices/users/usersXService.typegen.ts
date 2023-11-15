// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "": { type: "" };
    "done.invoke.activateUser": {
      type: "done.invoke.activateUser";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.deleteUser": {
      type: "done.invoke.deleteUser";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.getUsers": {
      type: "done.invoke.getUsers";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.resetUserPassword": {
      type: "done.invoke.resetUserPassword";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.suspendUser": {
      type: "done.invoke.suspendUser";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.updateUserRoles": {
      type: "done.invoke.updateUserRoles";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "error.platform.activateUser": {
      type: "error.platform.activateUser";
      data: unknown;
    };
    "error.platform.deleteUser": {
      type: "error.platform.deleteUser";
      data: unknown;
    };
    "error.platform.getUsers": {
      type: "error.platform.getUsers";
      data: unknown;
    };
    "error.platform.resetUserPassword": {
      type: "error.platform.resetUserPassword";
      data: unknown;
    };
    "error.platform.suspendUser": {
      type: "error.platform.suspendUser";
      data: unknown;
    };
    "error.platform.updateUserRoles": {
      type: "error.platform.updateUserRoles";
      data: unknown;
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    activateUser: "done.invoke.activateUser";
    deleteUser: "done.invoke.deleteUser";
    getUsers: "done.invoke.getUsers";
    resetUserPassword: "done.invoke.resetUserPassword";
    suspendUser: "done.invoke.suspendUser";
    updateUserRoles: "done.invoke.updateUserRoles";
  };
  missingImplementations: {
    actions: "updateURL";
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    assignActivateUserError: "error.platform.activateUser";
    assignDeleteUserError: "error.platform.deleteUser";
    assignFilter: "UPDATE_FILTER";
    assignGetUsersError: "error.platform.getUsers";
    assignPaginationRef: "UPDATE_PAGE" | "xstate.init";
    assignResetUserPasswordError: "error.platform.resetUserPassword";
    assignSuspendUserError: "error.platform.suspendUser";
    assignUpdateRolesError: "error.platform.updateUserRoles";
    assignUserIdToResetPassword: "RESET_USER_PASSWORD";
    assignUserIdToUpdateRoles: "UPDATE_USER_ROLES";
    assignUserToActivate: "ACTIVATE_USER";
    assignUserToDelete: "DELETE_USER";
    assignUserToSuspend: "SUSPEND_USER";
    assignUsers: "done.invoke.getUsers";
    clearActivateUserError: "CONFIRM_USER_ACTIVATION";
    clearDeleteUserError: "CONFIRM_USER_DELETE";
    clearGetUsersError:
      | ""
      | "UPDATE_PAGE"
      | "done.invoke.activateUser"
      | "done.invoke.deleteUser"
      | "done.invoke.suspendUser";
    clearResetUserPasswordError: "CONFIRM_USER_PASSWORD_RESET";
    clearSelectedUser:
      | "CANCEL_USER_ACTIVATION"
      | "CANCEL_USER_DELETE"
      | "CANCEL_USER_PASSWORD_RESET"
      | "CANCEL_USER_SUSPENSION"
      | "done.invoke.getUsers"
      | "done.invoke.resetUserPassword"
      | "done.invoke.updateUserRoles"
      | "error.platform.activateUser"
      | "error.platform.deleteUser"
      | "error.platform.getUsers"
      | "error.platform.resetUserPassword"
      | "error.platform.suspendUser"
      | "error.platform.updateUserRoles";
    clearSuspendUserError: "CONFIRM_USER_SUSPENSION";
    clearUpdateUserRolesError: "UPDATE_USER_ROLES";
    clearUsers: "error.platform.getUsers";
    displayActivateSuccess: "done.invoke.activateUser";
    displayActivatedErrorMessage: "error.platform.activateUser";
    displayDeleteErrorMessage: "error.platform.deleteUser";
    displayDeleteSuccess: "done.invoke.deleteUser";
    displayResetPasswordErrorMessage: "error.platform.resetUserPassword";
    displayResetPasswordSuccess: "done.invoke.resetUserPassword";
    displaySuspendSuccess: "done.invoke.suspendUser";
    displaySuspendedErrorMessage: "error.platform.suspendUser";
    displayUpdateRolesErrorMessage: "error.platform.updateUserRoles";
    generateRandomPassword: "RESET_USER_PASSWORD";
    sendResetPage: "UPDATE_FILTER";
    updateURL: "UPDATE_PAGE";
    updateUserRolesInTheList: "done.invoke.updateUserRoles";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {
    activateUser: "CONFIRM_USER_ACTIVATION";
    deleteUser: "CONFIRM_USER_DELETE";
    getUsers:
      | ""
      | "UPDATE_PAGE"
      | "done.invoke.activateUser"
      | "done.invoke.deleteUser"
      | "done.invoke.suspendUser";
    resetUserPassword: "CONFIRM_USER_PASSWORD_RESET";
    suspendUser: "CONFIRM_USER_SUSPENSION";
    updateUserRoles: "UPDATE_USER_ROLES";
  };
  matchesStates:
    | "activatingUser"
    | "confirmUserActivation"
    | "confirmUserDeletion"
    | "confirmUserPasswordReset"
    | "confirmUserSuspension"
    | "deletingUser"
    | "gettingUsers"
    | "idle"
    | "resettingUserPassword"
    | "startingPagination"
    | "suspendingUser"
    | "updatingUserRoles";
  tags: "loading";
}
