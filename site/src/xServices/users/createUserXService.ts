import { assign, createMachine } from "xstate";
import * as API from "../../api/api";
import * as TypesGen from "../../api/typesGenerated";
import { displaySuccess } from "../../components/GlobalSnackbar/utils";

export const Language = {
  createUserSuccess: "Successfully created user.",
};

export interface CreateUserContext {
  error?: unknown;
}

export type CreateUserEvent =
  | { type: "CREATE"; user: TypesGen.CreateUserRequest }
  | { type: "CANCEL_CREATE_USER" };

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
          data: TypesGen.User;
        };
      },
    },
    initial: "idle",
    states: {
      idle: {
        on: {
          CREATE: "creatingUser",
          CANCEL_CREATE_USER: { actions: ["clearError"] },
        },
      },
      creatingUser: {
        entry: "clearError",
        invoke: {
          src: "createUser",
          id: "createUser",
          onDone: {
            target: "idle",
            actions: ["displayCreateUserSuccess", "redirectToUsersPage"],
          },
          onError: {
            target: "idle",
            actions: ["assignError"],
          },
        },
        tags: "loading",
      },
    },
  },
  {
    services: {
      createUser: (_, event) => API.createUser(event.user),
    },
    actions: {
      assignError: assign({
        error: (_, event) => event.data,
      }),
      clearError: assign({ error: (_) => undefined }),
      displayCreateUserSuccess: () => {
        displaySuccess(Language.createUserSuccess);
      },
    },
  },
);
