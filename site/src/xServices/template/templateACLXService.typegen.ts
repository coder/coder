// This file was automatically generated. Edits will be overwritten

export interface Typegen0 {
  "@@xstate/typegen": true;
  internalEvents: {
    "done.invoke.templateACL.addingGroup:invocation[0]": {
      type: "done.invoke.templateACL.addingGroup:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.templateACL.addingUser:invocation[0]": {
      type: "done.invoke.templateACL.addingUser:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.templateACL.loading:invocation[0]": {
      type: "done.invoke.templateACL.loading:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.templateACL.removingGroup:invocation[0]": {
      type: "done.invoke.templateACL.removingGroup:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.templateACL.removingUser:invocation[0]": {
      type: "done.invoke.templateACL.removingUser:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.templateACL.updatingGroup:invocation[0]": {
      type: "done.invoke.templateACL.updatingGroup:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "done.invoke.templateACL.updatingUser:invocation[0]": {
      type: "done.invoke.templateACL.updatingUser:invocation[0]";
      data: unknown;
      __tip: "See the XState TS docs to learn how to strongly type this.";
    };
    "xstate.init": { type: "xstate.init" };
  };
  invokeSrcNameMap: {
    addGroup: "done.invoke.templateACL.addingGroup:invocation[0]";
    addUser: "done.invoke.templateACL.addingUser:invocation[0]";
    loadTemplateACL: "done.invoke.templateACL.loading:invocation[0]";
    removeGroup: "done.invoke.templateACL.removingGroup:invocation[0]";
    removeUser: "done.invoke.templateACL.removingUser:invocation[0]";
    updateGroup: "done.invoke.templateACL.updatingGroup:invocation[0]";
    updateUser: "done.invoke.templateACL.updatingUser:invocation[0]";
  };
  missingImplementations: {
    actions: never;
    delays: never;
    guards: never;
    services: never;
  };
  eventsCausingActions: {
    addGroupToTemplateACL: "done.invoke.templateACL.addingGroup:invocation[0]";
    addUserToTemplateACL: "done.invoke.templateACL.addingUser:invocation[0]";
    assignGroupToBeAdded: "ADD_GROUP";
    assignGroupToBeUpdated: "UPDATE_GROUP_ROLE";
    assignTemplateACL: "done.invoke.templateACL.loading:invocation[0]";
    assignUserToBeAdded: "ADD_USER";
    assignUserToBeUpdated: "UPDATE_USER_ROLE";
    clearGroupToBeUpdated: "done.invoke.templateACL.updatingGroup:invocation[0]";
    clearUserToBeUpdated: "done.invoke.templateACL.updatingUser:invocation[0]";
    displayRemoveGroupSuccessMessage: "done.invoke.templateACL.removingGroup:invocation[0]";
    displayRemoveUserSuccessMessage: "done.invoke.templateACL.removingUser:invocation[0]";
    displayUpdateGroupSuccessMessage: "done.invoke.templateACL.updatingGroup:invocation[0]";
    displayUpdateUserSuccessMessage: "done.invoke.templateACL.updatingUser:invocation[0]";
    removeGroupFromTemplateACL: "REMOVE_GROUP";
    removeUserFromTemplateACL: "REMOVE_USER";
    runAddGroupCallback: "done.invoke.templateACL.addingGroup:invocation[0]";
    runAddUserCallback: "done.invoke.templateACL.addingUser:invocation[0]";
    updateGroupOnTemplateACL: "done.invoke.templateACL.updatingGroup:invocation[0]";
    updateUserOnTemplateACL: "done.invoke.templateACL.updatingUser:invocation[0]";
  };
  eventsCausingDelays: {};
  eventsCausingGuards: {};
  eventsCausingServices: {
    addGroup: "ADD_GROUP";
    addUser: "ADD_USER";
    loadTemplateACL: "xstate.init";
    removeGroup: "REMOVE_GROUP";
    removeUser: "REMOVE_USER";
    updateGroup: "UPDATE_GROUP_ROLE";
    updateUser: "UPDATE_USER_ROLE";
  };
  matchesStates:
    | "addingGroup"
    | "addingUser"
    | "idle"
    | "loading"
    | "removingGroup"
    | "removingUser"
    | "updatingGroup"
    | "updatingUser";
  tags: never;
}
