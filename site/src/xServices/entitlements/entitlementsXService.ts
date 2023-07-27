import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { Entitlements } from "../../api/typesGenerated"

export type EntitlementsContext = {
  entitlements?: Entitlements
  getEntitlementsError?: unknown
}

export const entitlementsMachine = createMachine(
  {
    id: "entitlementsMachine",
    predictableActionArguments: true,
    tsTypes: {} as import("./entitlementsXService.typegen").Typegen0,
    schema: {
      context: {} as EntitlementsContext,
      services: {
        getEntitlements: {
          data: {} as Entitlements,
        },
      },
    },
    initial: "gettingEntitlements",
    states: {
      gettingEntitlements: {
        entry: "clearGetEntitlementsError",
        invoke: {
          id: "getEntitlements",
          src: "getEntitlements",
          onDone: {
            target: "success",
            actions: ["assignEntitlements"],
          },
          onError: {
            target: "error",
            actions: ["assignGetEntitlementsError"],
          },
        },
      },
      success: {
        type: "final",
      },
      error: {
        type: "final",
      },
    },
  },
  {
    actions: {
      assignEntitlements: assign({
        entitlements: (_, event) => event.data,
      }),
      assignGetEntitlementsError: assign({
        getEntitlementsError: (_, event) => event.data,
      }),
      clearGetEntitlementsError: assign({
        getEntitlementsError: (_) => undefined,
      }),
    },
    services: {
      getEntitlements: async () => {
        // Entitlements is injected by the Coder server into the HTML document.
        const entitlements = document.querySelector(
          "meta[property=entitlements]",
        )
        if (entitlements) {
          const rawContent = entitlements.getAttribute("content")
          try {
            return JSON.parse(rawContent as string)
          } catch (ex) {
            // Ignore this and fetch as normal!
          }
        }

        return API.getEntitlements()
      },
    },
  },
)
