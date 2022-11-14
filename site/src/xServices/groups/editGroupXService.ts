import { getGroup, patchGroup } from "api/api"
import {
  ApiError,
  getErrorMessage,
  hasApiFieldErrors,
  isApiError,
  mapApiErrorToFieldErrors,
} from "api/errors"
import { Group } from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"

export const editGroupMachine = createMachine(
  {
    id: "editGroup",
    schema: {
      context: {} as {
        groupId: string
        group?: Group
        updateGroupFormErrors?: unknown
      },
      services: {} as {
        loadGroup: {
          data: Group
        }
        updateGroup: {
          data: Group
        }
      },
      events: {} as {
        type: "UPDATE"
        data: { name: string; avatar_url: string; quota_allowance: number }
      },
    },
    tsTypes: {} as import("./editGroupXService.typegen").Typegen0,
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
          UPDATE: {
            target: "updating",
          },
        },
      },
      updating: {
        invoke: {
          src: "updateGroup",
          onDone: {
            actions: ["onUpdate"],
          },
          onError: [
            {
              target: "idle",
              cond: "hasFieldErrors",
              actions: ["assignUpdateGroupFormErrors"],
            },
            {
              target: "idle",
              actions: ["displayUpdateGroupError"],
            },
          ],
        },
      },
    },
  },
  {
    guards: {
      hasFieldErrors: (_, event) =>
        isApiError(event.data) && hasApiFieldErrors(event.data),
    },
    services: {
      loadGroup: ({ groupId }) => getGroup(groupId),

      updateGroup: ({ group }, { data }) => {
        if (!group) {
          throw new Error("Group not defined.")
        }

        return patchGroup(group.id, {
          ...data,
          add_users: [],
          remove_users: [],
        })
      },
    },
    actions: {
      assignGroup: assign({
        group: (_, { data }) => data,
      }),
      displayLoadGroupError: (_, { data }) => {
        const message = getErrorMessage(data, "Failed to the group.")
        displayError(message)
      },
      displayUpdateGroupError: (_, { data }) => {
        const message = getErrorMessage(data, "Failed to update the group.")
        displayError(message)
      },
      assignUpdateGroupFormErrors: (_, event) =>
        mapApiErrorToFieldErrors((event.data as ApiError).response.data),
    },
  },
)
