import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { Entitlements } from "../../api/types"

export const Language = {
  getEntitlementsError: "Error getting license entitlements.",
}

export type EntitlementsContext = {
  entitlements: Entitlements
  getEntitlementsError?: Error | unknown
}

export type EntitlementsEvent = {
  type: "GET_ENTITLEMENTS"
}

export const entitlementsMachine = createMachine(
  {
    id: "entitlementsMachine",
    initial: "idle",
    schema: {
      context: {} as EntitlementsContext,
      events: {} as EntitlementsEvent,
      services: {
        getEntitlements: {
          data: {} as Entitlements,
        },
      },
    },
    tsTypes: {} as import("./entitlementsXService.typegen").Typegen0,
    context: {
      entitlements: {
        warnings: [],
        features: {},
        has_license: false,
      },
    },
    states: {
      idle: {
        on: {
          GET_ENTITLEMENTS: "gettingEntitlements",
        },
      },
      gettingEntitlements: {
        entry: "clearGetEntitlementsError",
        invoke: {
          id: "getEntitlements",
          src: "getEntitlements",
          onDone: {
            target: "idle",
            actions: ["assignEntitlements"],
          },
          onError: {
            target: "idle",
            actions: ["assignGetEntitlementsError"],
          },
        },
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
      getEntitlements: () => API.getEntitlements(),
    },
  },
)
