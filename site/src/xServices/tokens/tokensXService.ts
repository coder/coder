import { getTokens, deleteAPIKey } from "api/api"
import { APIKey } from "api/typesGenerated"
import { displaySuccess } from "components/GlobalSnackbar/utils"
import { createMachine, assign } from "xstate"

interface Context {
  tokens?: APIKey[]
  getTokensError?: unknown
  deleteTokenError?: unknown
  deleteTokenId?: string
}

type Events =
  | { type: "DELETE_TOKEN"; id: string }
  | { type: "CONFIRM_DELETE_TOKEN" }
  | { type: "CANCEL_DELETE_TOKEN" }

const Language = {
  deleteSuccess: "Token has been deleted",
}

export const tokensMachine = createMachine(
  {
    id: "tokensState",
    predictableActionArguments: true,
    schema: {
      context: {} as Context,
      events: {} as Events,
      services: {} as {
        getTokens: {
          data: APIKey[]
        }
        deleteToken: {
          data: unknown
        }
      },
    },
    tsTypes: {} as import("./tokensXService.typegen").Typegen0,
    initial: "gettingTokens",
    states: {
      gettingTokens: {
        entry: "clearGetTokensError",
        invoke: {
          src: "getTokens",
          onDone: [
            {
              actions: "assignTokens",
              target: "loaded",
            },
          ],
          onError: [
            {
              actions: "assignGetTokensError",
              target: "notLoaded",
            },
          ],
        },
      },
      notLoaded: {
        type: "final",
      },
      loaded: {
        on: {
          DELETE_TOKEN: {
            actions: "assignDeleteTokenId",
            target: "confirmTokenDelete",
          },
        },
      },
      confirmTokenDelete: {
        on: {
          CANCEL_DELETE_TOKEN: {
            actions: "clearDeleteTokenId",
            target: "loaded",
          },
          CONFIRM_DELETE_TOKEN: {
            target: "deletingToken",
          },
        },
      },
      deletingToken: {
        entry: "clearDeleteTokenError",
        invoke: {
          src: "deleteToken",
          onDone: [
            {
              actions: ["clearDeleteTokenId", "notifySuccessTokenDeleted"],
              target: "gettingTokens",
            },
          ],
          onError: [
            {
              actions: ["clearDeleteTokenId", "assignDeleteTokenError"],
              target: "loaded",
            },
          ],
        },
      },
    },
  },
  {
    services: {
      getTokens: () => getTokens(),
      deleteToken: (context) => {
        if (context.deleteTokenId === undefined) {
          return Promise.reject("No token id to delete")
        }

        return deleteAPIKey(context.deleteTokenId)
      },
    },
    actions: {
      assignTokens: assign({
        tokens: (_, { data }) => data,
      }),
      assignGetTokensError: assign({
        getTokensError: (_, { data }) => data,
      }),
      clearGetTokensError: assign({
        getTokensError: (_) => undefined,
      }),
      assignDeleteTokenId: assign({
        deleteTokenId: (_, event) => event.id,
      }),
      clearDeleteTokenId: assign({
        deleteTokenId: (_) => undefined,
      }),
      assignDeleteTokenError: assign({
        deleteTokenError: (_, { data }) => data,
      }),
      clearDeleteTokenError: assign({
        deleteTokenError: (_) => undefined,
      }),
      notifySuccessTokenDeleted: () => {
        displaySuccess(Language.deleteSuccess)
      },
    },
  },
)
