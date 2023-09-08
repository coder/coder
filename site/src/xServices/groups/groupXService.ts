import { deleteGroup, getGroup, patchGroup, checkAuthorization } from "api/api";
import { getErrorMessage } from "api/errors";
import { AuthorizationResponse, Group } from "api/typesGenerated";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { assign, createMachine } from "xstate";

export const groupMachine = createMachine(
  {
    id: "group",
    schema: {
      context: {} as {
        groupId: string;
        group?: Group;
        permissions?: AuthorizationResponse;
        addMemberCallback?: () => void;
        removingMember?: string;
      },
      services: {} as {
        loadGroup: {
          data: Group;
        };
        loadPermissions: {
          data: AuthorizationResponse;
        };
        addMember: {
          data: Group;
        };
        removeMember: {
          data: Group;
        };
        deleteGroup: {
          data: unknown;
        };
      },
      events: {} as
        | {
            type: "ADD_MEMBER";
            userId: string;
            callback: () => void;
          }
        | {
            type: "REMOVE_MEMBER";
            userId: string;
          }
        | {
            type: "DELETE";
          }
        | {
            type: "CONFIRM_DELETE";
          }
        | {
            type: "CANCEL_DELETE";
          },
    },
    tsTypes: {} as import("./groupXService.typegen").Typegen0,
    initial: "loading",
    states: {
      loading: {
        type: "parallel",
        states: {
          data: {
            initial: "loading",
            states: {
              loading: {
                invoke: {
                  src: "loadGroup",
                  onDone: {
                    actions: ["assignGroup"],
                    target: "success",
                  },
                  onError: {
                    actions: ["displayLoadGroupError"],
                  },
                },
              },
              success: {
                type: "final",
              },
            },
          },
          permissions: {
            initial: "loading",
            states: {
              loading: {
                invoke: {
                  src: "loadPermissions",
                  onDone: {
                    actions: ["assignPermissions"],
                    target: "success",
                  },
                  onError: {
                    actions: ["displayLoadPermissionsError"],
                  },
                },
              },
              success: {
                type: "final",
              },
            },
          },
        },
        onDone: "idle",
      },
      idle: {
        on: {
          ADD_MEMBER: {
            target: "addingMember",
            actions: ["assignAddMemberCallback"],
          },
          REMOVE_MEMBER: {
            target: "removingMember",
            actions: ["removeUserFromMembers"],
          },
          DELETE: {
            target: "confirmingDelete",
          },
        },
      },
      addingMember: {
        invoke: {
          src: "addMember",
          onDone: {
            actions: ["assignGroup", "callAddMemberCallback"],
            target: "idle",
          },
          onError: {
            target: "idle",
            actions: ["displayAddMemberError"],
          },
        },
      },
      removingMember: {
        invoke: {
          src: "removeMember",
          onDone: {
            actions: ["assignGroup", "displayRemoveMemberSuccess"],
            target: "idle",
          },
          onError: {
            target: "idle",
            actions: ["displayRemoveMemberError"],
          },
        },
      },
      confirmingDelete: {
        on: {
          CONFIRM_DELETE: "deleting",
          CANCEL_DELETE: "idle",
        },
      },
      deleting: {
        invoke: {
          src: "deleteGroup",
          onDone: {
            actions: ["redirectToGroups"],
          },
          onError: {
            actions: ["displayDeleteGroupError"],
          },
        },
      },
    },
  },
  {
    services: {
      loadGroup: ({ groupId }) => getGroup(groupId),
      loadPermissions: ({ groupId }) =>
        checkAuthorization({
          checks: {
            canUpdateGroup: {
              object: {
                resource_type: "group",
                resource_id: groupId,
              },
              action: "update",
            },
          },
        }),
      addMember: ({ group }, { userId }) => {
        if (!group) {
          throw new Error("Group not defined.");
        }

        return patchGroup(group.id, {
          name: "",
          display_name: "",
          add_users: [userId],
          remove_users: [],
        });
      },
      removeMember: ({ group }, { userId }) => {
        if (!group) {
          throw new Error("Group not defined.");
        }

        return patchGroup(group.id, {
          name: "",
          display_name: "",
          add_users: [],
          remove_users: [userId],
        });
      },
      deleteGroup: ({ group }) => {
        if (!group) {
          throw new Error("Group not defined.");
        }

        return deleteGroup(group.id);
      },
    },
    actions: {
      assignGroup: assign({
        group: (_, { data }) => data,
      }),
      assignAddMemberCallback: assign({
        addMemberCallback: (_, { callback }) => callback,
      }),
      displayLoadGroupError: (_, { data }) => {
        const message = getErrorMessage(data, "Failed to load the group.");
        displayError(message);
      },
      displayAddMemberError: (_, { data }) => {
        const message = getErrorMessage(
          data,
          "Failed to add member to the group.",
        );
        displayError(message);
      },
      callAddMemberCallback: ({ addMemberCallback }) => {
        if (addMemberCallback) {
          addMemberCallback();
        }
      },
      // Optimistically remove the user from members
      removeUserFromMembers: assign({
        group: ({ group }, { userId }) => {
          if (!group) {
            throw new Error("Group is not defined.");
          }

          return {
            ...group,
            members: group.members.filter(
              (currentMember) => currentMember.id !== userId,
            ),
          };
        },
      }),
      displayRemoveMemberError: (_, { data }) => {
        const message = getErrorMessage(
          data,
          "Failed to remove member from the group.",
        );
        displayError(message);
      },
      displayRemoveMemberSuccess: () => {
        displaySuccess("Member removed successfully.");
      },
      displayDeleteGroupError: (_, { data }) => {
        const message = getErrorMessage(data, "Failed to delete group.");
        displayError(message);
      },
      assignPermissions: assign({
        permissions: (_, { data }) => data,
      }),
      displayLoadPermissionsError: (_, { data }) => {
        const message = getErrorMessage(
          data,
          "Failed to load the permissions.",
        );
        displayError(message);
      },
    },
  },
);
