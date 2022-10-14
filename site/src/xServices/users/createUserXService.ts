import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import {
  ApiError,
  FieldErrors,
  getErrorMessage,
  hasApiFieldErrors,
  isApiError,
  mapApiErrorToFieldErrors,
} from "../../api/errors"
import * as TypesGen from "../../api/typesGenerated"
import { displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  createUserSuccess: "Successfully created user.",
  createUserError: "Error on creating the user.",
}

export interface CreateUserContext {
  createUserErrorMessage?: string
  createUserFormErrors?: FieldErrors
}

export type CreateUserEvent =
  | { type: "CREATE"; user: TypesGen.CreateUserRequest }
  | { type: "CANCEL_CREATE_USER" }

export const createUserMachine = createMachine(
  {
    id: "usersState",
    predictableActionArguments: true,
    tsTypes: {} as import("./createUserXService.typegen").Typegen0,
    schema: {
      context: {} as CreateUserContext,
      events: {} as CreateUserEvent,
      services: {} as {
        createUser: {
          data: TypesGen.User
        }
      },
    },
    initial: "idle",
    states: {
      idle: {
        on: {
          CREATE: "creatingUser",
          CANCEL_CREATE_USER: { actions: ["clearCreateUserError"] },
        },
      },
      creatingUser: {
        entry: "clearCreateUserError",
        invoke: {
          src: "createUser",
          id: "createUser",
          onDone: {
            target: "idle",
            actions: ["displayCreateUserSuccess", "redirectToUsersPage"],
          },
          onError: [
            {
              target: "idle",
              cond: "hasFieldErrors",
              actions: ["assignCreateUserFormErrors"],
            },
            {
              target: "idle",
              actions: ["assignCreateUserError"],
            },
          ],
        },
        tags: "loading",
      },
    },
  },
  {
    services: {
      createUser: (_, event) => API.createUser(event.user),
    },
    guards: {
      hasFieldErrors: (_, event) =>
        isApiError(event.data) && hasApiFieldErrors(event.data),
    },
    actions: {
      assignCreateUserError: assign({
        createUserErrorMessage: (_, event) =>
          getErrorMessage(event.data, Language.createUserError),
      }),
      assignCreateUserFormErrors: assign({
        // the guard ensures it is ApiError
        createUserFormErrors: (_, event) =>
          mapApiErrorToFieldErrors((event.data as ApiError).response.data),
      }),
      clearCreateUserError: assign((context: CreateUserContext) => ({
        ...context,
        createUserErrorMessage: undefined,
        createUserFormErrors: undefined,
      })),
      displayCreateUserSuccess: () => {
        displaySuccess(Language.createUserSuccess)
      },
    },
  },
)
