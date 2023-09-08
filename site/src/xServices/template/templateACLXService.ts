import { getTemplateACL, updateTemplateACL } from "api/api";
import {
  TemplateACL,
  TemplateGroup,
  TemplateRole,
  TemplateUser,
} from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { assign, createMachine } from "xstate";

export const templateACLMachine = createMachine(
  {
    schema: {
      context: {} as {
        templateId: string;
        templateACL?: TemplateACL;
        // User
        userToBeAdded?: TemplateUser;
        userToBeUpdated?: TemplateUser;
        addUserCallback?: () => void;
        // Group
        groupToBeAdded?: TemplateGroup;
        groupToBeUpdated?: TemplateGroup;
        addGroupCallback?: () => void;
      },
      services: {} as {
        loadTemplateACL: {
          data: TemplateACL;
        };
        // User
        addUser: {
          data: unknown;
        };
        updateUser: {
          data: unknown;
        };
        // Group
        addGroup: {
          data: unknown;
        };
        updateGroup: {
          data: unknown;
        };
      },
      events: {} as  // User
        | {
            type: "ADD_USER";
            user: TemplateUser;
            role: TemplateRole;
            onDone: () => void;
          }
        | {
            type: "UPDATE_USER_ROLE";
            user: TemplateUser;
            role: TemplateRole;
          }
        | {
            type: "REMOVE_USER";
            user: TemplateUser;
          }
        // Group
        | {
            type: "ADD_GROUP";
            group: TemplateGroup;
            role: TemplateRole;
            onDone: () => void;
          }
        | {
            type: "UPDATE_GROUP_ROLE";
            group: TemplateGroup;
            role: TemplateRole;
          }
        | {
            type: "REMOVE_GROUP";
            group: TemplateGroup;
          },
    },
    tsTypes: {} as import("./templateACLXService.typegen").Typegen0,
    id: "templateACL",
    initial: "loading",
    states: {
      loading: {
        invoke: {
          src: "loadTemplateACL",
          onDone: {
            actions: ["assignTemplateACL"],
            target: "idle",
          },
        },
      },
      idle: {
        on: {
          // User
          ADD_USER: { target: "addingUser", actions: ["assignUserToBeAdded"] },
          UPDATE_USER_ROLE: {
            target: "updatingUser",
            actions: ["assignUserToBeUpdated"],
          },
          REMOVE_USER: {
            target: "removingUser",
            actions: ["removeUserFromTemplateACL"],
          },
          // Group
          ADD_GROUP: {
            target: "addingGroup",
            actions: ["assignGroupToBeAdded"],
          },
          UPDATE_GROUP_ROLE: {
            target: "updatingGroup",
            actions: ["assignGroupToBeUpdated"],
          },
          REMOVE_GROUP: {
            target: "removingGroup",
            actions: ["removeGroupFromTemplateACL"],
          },
        },
      },
      // User
      addingUser: {
        invoke: {
          src: "addUser",
          onDone: {
            target: "idle",
            actions: ["addUserToTemplateACL", "runAddUserCallback"],
          },
        },
      },
      updatingUser: {
        invoke: {
          src: "updateUser",
          onDone: {
            target: "idle",
            actions: [
              "updateUserOnTemplateACL",
              "clearUserToBeUpdated",
              "displayUpdateUserSuccessMessage",
            ],
          },
        },
      },
      removingUser: {
        invoke: {
          src: "removeUser",
          onDone: {
            target: "idle",
            actions: ["displayRemoveUserSuccessMessage"],
          },
        },
      },
      // Group
      addingGroup: {
        invoke: {
          src: "addGroup",
          onDone: {
            target: "idle",
            actions: ["addGroupToTemplateACL", "runAddGroupCallback"],
          },
        },
      },
      updatingGroup: {
        invoke: {
          src: "updateGroup",
          onDone: {
            target: "idle",
            actions: [
              "updateGroupOnTemplateACL",
              "clearGroupToBeUpdated",
              "displayUpdateGroupSuccessMessage",
            ],
          },
        },
      },
      removingGroup: {
        invoke: {
          src: "removeGroup",
          onDone: {
            target: "idle",
            actions: ["displayRemoveGroupSuccessMessage"],
          },
        },
      },
    },
  },
  {
    services: {
      loadTemplateACL: ({ templateId }) => getTemplateACL(templateId),
      // User
      addUser: ({ templateId }, { user, role }) =>
        updateTemplateACL(templateId, {
          user_perms: {
            [user.id]: role,
          },
        }),
      updateUser: ({ templateId }, { user, role }) =>
        updateTemplateACL(templateId, {
          user_perms: {
            [user.id]: role,
          },
        }),
      removeUser: ({ templateId }, { user }) =>
        updateTemplateACL(templateId, {
          user_perms: {
            [user.id]: "",
          },
        }),
      // Group
      addGroup: ({ templateId }, { group, role }) =>
        updateTemplateACL(templateId, {
          group_perms: {
            [group.id]: role,
          },
        }),
      updateGroup: ({ templateId }, { group, role }) =>
        updateTemplateACL(templateId, {
          group_perms: {
            [group.id]: role,
          },
        }),
      removeGroup: ({ templateId }, { group }) =>
        updateTemplateACL(templateId, {
          group_perms: {
            [group.id]: "",
          },
        }),
    },
    actions: {
      assignTemplateACL: assign({
        templateACL: (_, { data }) => data,
      }),
      // User
      assignUserToBeAdded: assign({
        userToBeAdded: (_, { user, role }) => ({ ...user, role }),
        addUserCallback: (_, { onDone }) => onDone,
      }),
      addUserToTemplateACL: assign({
        templateACL: ({ templateACL, userToBeAdded }) => {
          if (!userToBeAdded) {
            throw new Error("No user to be added");
          }
          if (!templateACL) {
            throw new Error("Template ACL is not loaded yet");
          }
          return {
            ...templateACL,
            users: [...templateACL.users, userToBeAdded],
          };
        },
      }),
      runAddUserCallback: ({ addUserCallback }) => {
        if (addUserCallback) {
          addUserCallback();
        }
      },
      assignUserToBeUpdated: assign({
        userToBeUpdated: (_, { user, role }) => ({ ...user, role }),
      }),
      updateUserOnTemplateACL: assign({
        templateACL: ({ templateACL, userToBeUpdated }) => {
          if (!userToBeUpdated) {
            throw new Error("No user to be added");
          }
          if (!templateACL) {
            throw new Error("Template ACL is not loaded yet");
          }
          return {
            ...templateACL,
            users: templateACL.users.map((oldTemplateUser) => {
              return oldTemplateUser.id === userToBeUpdated.id
                ? userToBeUpdated
                : oldTemplateUser;
            }),
          };
        },
      }),
      clearUserToBeUpdated: assign({
        userToBeUpdated: (_) => undefined,
      }),
      displayUpdateUserSuccessMessage: () => {
        displaySuccess("User role update successfully!");
      },
      removeUserFromTemplateACL: assign({
        templateACL: ({ templateACL }, { user }) => {
          if (!templateACL) {
            throw new Error("Template ACL is not loaded yet");
          }
          return {
            ...templateACL,
            users: templateACL.users.filter((oldTemplateUser) => {
              return oldTemplateUser.id !== user.id;
            }),
          };
        },
      }),
      displayRemoveUserSuccessMessage: () => {
        displaySuccess("User removed successfully!");
      },
      // Group
      assignGroupToBeAdded: assign({
        groupToBeAdded: (_, { group, role }) => ({ ...group, role }),
        addGroupCallback: (_, { onDone }) => onDone,
      }),
      addGroupToTemplateACL: assign({
        templateACL: ({ templateACL, groupToBeAdded }) => {
          if (!groupToBeAdded) {
            throw new Error("No group to be added");
          }
          if (!templateACL) {
            throw new Error("Template ACL is not loaded yet");
          }
          return {
            ...templateACL,
            group: [...templateACL.group, groupToBeAdded],
          };
        },
      }),
      runAddGroupCallback: ({ addGroupCallback }) => {
        if (addGroupCallback) {
          addGroupCallback();
        }
      },
      assignGroupToBeUpdated: assign({
        groupToBeUpdated: (_, { group, role }) => ({ ...group, role }),
      }),
      updateGroupOnTemplateACL: assign({
        templateACL: ({ templateACL, groupToBeUpdated }) => {
          if (!groupToBeUpdated) {
            throw new Error("No group to be added");
          }
          if (!templateACL) {
            throw new Error("Template ACL is not loaded yet");
          }
          return {
            ...templateACL,
            group: templateACL.group.map((oldTemplateGroup) => {
              return oldTemplateGroup.id === groupToBeUpdated.id
                ? groupToBeUpdated
                : oldTemplateGroup;
            }),
          };
        },
      }),
      clearGroupToBeUpdated: assign({
        groupToBeUpdated: (_) => undefined,
      }),
      displayUpdateGroupSuccessMessage: () => {
        displaySuccess("Group role update successfully!");
      },
      removeGroupFromTemplateACL: assign({
        templateACL: ({ templateACL }, { group }) => {
          if (!templateACL) {
            throw new Error("Template ACL is not loaded yet");
          }
          return {
            ...templateACL,
            group: templateACL.group.filter((oldTemplateGroup) => {
              return oldTemplateGroup.id !== group.id;
            }),
          };
        },
      }),
      displayRemoveGroupSuccessMessage: () => {
        displaySuccess("Group removed successfully!");
      },
    },
  },
);
