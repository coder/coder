import { assign, createMachine } from "xstate";
import * as TypeGen from "api/typesGenerated";
import * as API from "api/api";

export interface AuthMethodsContext {
  authMethods?: TypeGen.AuthMethods;
  error?: unknown;
}

export const authMethodsXService = createMachine(
  {
    id: "authMethods",
    predictableActionArguments: true,
    tsTypes: {} as import("./authMethodsXService.typegen").Typegen0,
    schema: {
      context: {} as AuthMethodsContext,
      services: {} as {
        getAuthMethods: {
          data: TypeGen.AuthMethods;
        };
      },
    },
    context: {},
    initial: "gettingAuthMethods",
    states: {
      gettingAuthMethods: {
        invoke: {
          src: "getAuthMethods",
          onDone: {
            target: "idle",
            actions: ["assignAuthMethods"],
          },
          onError: {
            target: "idle",
            actions: ["setError"],
          },
        },
      },
      idle: {},
    },
  },
  {
    actions: {
      assignAuthMethods: assign({
        authMethods: (_, event) => event.data,
      }),
      setError: assign({
        error: (_, event) => event.data,
      }),
    },
    services: {
      getAuthMethods: () => API.getAuthMethods(),
    },
  },
);
