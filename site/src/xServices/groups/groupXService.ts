import { getGroup, patchGroup } from "api/api"
import { getErrorMessage } from "api/errors"
import { Group } from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"

export const groupMachine = createMachine(
  {
    id: "group",
    schema: {
      context: {} as {
        groupId: string
        group?: Group
        addMemberCallback?: () => void
      },
      services: {} as {
        loadGroup: {
          data: Group
        }
        addMember: {
          data: Group
        }
      },
      events: {} as {
        type: "ADD_MEMBER"
        userId: string
        callback: () => void
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
    },
  },
)
