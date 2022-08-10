import * as API from "api/api"
import {
  ApiError,
  FieldErrors,
  getErrorMessage,
  hasApiFieldErrors,
  isApiError,
  mapApiErrorToFieldErrors,
} from "api/errors"
import * as TypesGen from "api/typesGenerated"
import { assign, createMachine } from "xstate"

export const Language = {
  createFirstUserError: "Error on creating the user.",
}

export interface SetupContext {
  createFirstUserErrorMessage?: string
  createFirstUserFormErrors?: FieldErrors
}

export type SetupEvent = { type: "CREATE_FIRST_USER"; firstUser: TypesGen.CreateFirstUserRequest }

export const setupMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QGUwBcCuAHZaCGaYAdAJYQA2YAxAMIBKAogIIAqDA+gGICSdyL7AKrIGdRKCwB7WCTQlJAO3EgAHogDMAViKaAnPoDsABgBMRgCy6jpkwBoQAT0QBGI+qIA2N0ePOAHJqa-qYAviH2qJg4+IREAMYATmAEJApQnCQJsGiCsGAJVBCKxKkAbpIA1sSJyYQZWTl5CcpSMnKKymoI6ibuzs4efh6a6h4eJiMj9k4I5uZ+RPNjfhPOliZW5mER6Ni4BNVJKWn12bn5VPkJkglEWOQEAGY3ALbxR3WZZ00t0rLySiQqg0uncHmcJnMxj8WmsWnU0xcYyIA3UUJ8-V84zC4RACkkEDgykiexiJQoYF+bQBnUQflcRAMZlc6lGJgMBj8iIQZm0lj8BmcugMVjcfnm2xAJOiB3etVS6S+jXyVP+HSBXQAtLptMzOVp-Op+rpnNyjboiKZ1AZrTbzEY-H5hZLpftYo8lecEjQPpBVe1AaAuoELaz5rbRlYxtzzJDLSYIaCBr0bC7djLCP6aRrEJrjUQ9TCgjDjabHBpnJ5vJoDLHdJZWUZNDiQkA */
  createMachine(
    {
      tsTypes: {} as import("./setupXService.typegen").Typegen0,
      schema: {
        context: {} as SetupContext,
        events: {} as SetupEvent,
        services: {} as {
          createFirstUser: {
            data: TypesGen.CreateFirstUserResponse
          }
        },
      },
      id: "SetupState",
      initial: "idle",
      states: {
        idle: {
          on: {
            CREATE_FIRST_USER: {
              target: "creatingFirstUser",
            },
          },
        },
        creatingFirstUser: {
          entry: "clearCreateFirstUserError",
          invoke: {
            src: "createFirstUser",
            id: "createFirstUser",
            onDone: [
              {
                target: "firstUserCreated",
              },
            ],
            onError: [
              {
                actions: "assignCreateFirstUserFormErrors",
                cond: "hasFieldErrors",
                target: "idle",
              },
              {
                actions: "assignCreateFirstUserError",
                target: "idle",
              },
            ],
          },
          tags: "loading",
        },
        firstUserCreated: {
          entry: "redirectToWorkspacesPage",
          type: "final",
        },
      },
    },
    {
      services: {
        createFirstUser: (_, event) => API.createFirstUser(event.firstUser),
      },
      guards: {
        hasFieldErrors: (_, event) => isApiError(event.data) && hasApiFieldErrors(event.data),
      },
      actions: {
        assignCreateFirstUserError: assign({
          createFirstUserErrorMessage: (_, event) =>
            getErrorMessage(event.data, Language.createFirstUserError),
        }),
        assignCreateFirstUserFormErrors: assign({
          // the guard ensures it is ApiError
          createFirstUserFormErrors: (_, event) =>
            mapApiErrorToFieldErrors((event.data as ApiError).response.data),
        }),

        clearCreateFirstUserError: assign((context: SetupContext) => ({
          ...context,
          createFirstUserErrorMessage: undefined,
          createFirstUserFormErrors: undefined,
        })),
      },
    },
  )
