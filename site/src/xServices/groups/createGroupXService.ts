import { createGroup } from "api/api"
import {
  ApiError,
  getErrorMessage,
  hasApiFieldErrors,
  isApiError,
  mapApiErrorToFieldErrors,
} from "api/errors"
import { CreateGroupRequest, Group } from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
import { createMachine } from "xstate"

export const createGroupMachine = createMachine(
  {
    id: "createGroupMachine",
    schema: {
      context: {} as {
        organizationId: string
        createGroupFormErrors?: unknown
      },
      services: {} as {
        createGroup: {
          data: Group
        }
      },
      events: {} as {
        type: "CREATE"
        data: CreateGroupRequest
      },
    },
    tsTypes: {} as import("./createGroupXService.typegen").Typegen0,
    initial: "idle",
    states: {
      idle: {
        on: {
          CREATE: {
            target: "creatingGroup",
          },
        },
      },
      creatingGroup: {
        invoke: {
          src: "createGroup",
          onDone: {
            target: "idle",
            actions: ["onCreate"],
          },
          onError: [
            {
              target: "idle",
              cond: "hasFieldErrors",
              actions: ["assignCreateGroupFormErrors"],
            },
            {
              target: "idle",
              actions: ["displayCreateGroupError"],
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
      createGroup: ({ organizationId }, { data }) =>
        createGroup(organizationId, data),
    },
    actions: {
      displayCreateGroupError: (_, { data }) => {
        const message = getErrorMessage(data, "Error on creating the group.")
        displayError(message)
      },
      assignCreateGroupFormErrors: (_, event) =>
        mapApiErrorToFieldErrors((event.data as ApiError).response.data),
    },
  },
)
