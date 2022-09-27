import { getGroup, patchGroup } from "api/api"
import { getErrorMessage } from "api/errors"
import { Group } from "api/typesGenerated"
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"

export const groupMachine = createMachine(
  {
    id: "group",
    schema: {
      context: {} as {
        groupId: string
        group?: Group
        addMemberCallback?: () => void
        removingMember?: string
      },
      services: {} as {
        loadGroup: {
          data: Group
        }
        addMember: {
          data: Group
        }
        removeMember: {
          data: Group
        }
      },
      events: {} as
        | {
            type: "ADD_MEMBER"
            userId: string
            callback: () => void
          }
        | {
            type: "REMOVE_MEMBER"
            userId: string
          },
    },
    tsTypes: {} as import("./groupXService.typegen").Typegen0,
    initial: "loading",
    states: {
      loading: {
        invoke: {
          src: "loadGroup",
          onDone: {
            actions: ["assignGroup"],
            target: "idle",
          },
          onError: {
            actions: ["displayLoadGroupError"],
            target: "idle",
          },
        },
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
    },
  },
  {
    services: {
      loadGroup: ({ groupId }) => getGroup(groupId),
      addMember: ({ group }, { userId }) => {
        if (!group) {
          throw new Error("Group not defined.")
        }

        return patchGroup(group.id, { name: "", add_users: [userId], remove_users: [] })
      },
      removeMember: ({ group }, { userId }) => {
        if (!group) {
          throw new Error("Group not defined.")
        }

        return patchGroup(group.id, { name: "", add_users: [], remove_users: [userId] })
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
        const message = getErrorMessage(data, "Failed to the group.")
        displayError(message)
      },
      displayAddMemberError: (_, { data }) => {
        const message = getErrorMessage(data, "Failed to add member to the group.")
        displayError(message)
      },
      callAddMemberCallback: ({ addMemberCallback }) => {
        if (addMemberCallback) {
          addMemberCallback()
        }
      },
      // Optimistically remove the user from members
      removeUserFromMembers: assign({
        group: ({ group }, { userId }) => {
          if (!group) {
            throw new Error("Group is not defined.")
          }

          return {
            ...group,
            members: group.members.filter((currentMember) => currentMember.id !== userId),
          }
        },
      }),
      displayRemoveMemberError: (_, { data }) => {
        const message = getErrorMessage(data, "Failed to remove member from the group.")
        displayError(message)
      },
      displayRemoveMemberSuccess: () => {
        displaySuccess("Member removed successfully.")
      },
    },
  },
)
