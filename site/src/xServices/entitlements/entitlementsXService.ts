import { MockEntitlementsWithWarnings } from "testHelpers/entities"
import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import { Entitlements } from "../../api/typesGenerated"

export const Language = {
  getEntitlementsError: "Error getting license entitlements.",
}

export type EntitlementsContext = {
  entitlements: Entitlements
  getEntitlementsError?: Error | unknown
}

export type EntitlementsEvent =
  | {
      type: "GET_ENTITLEMENTS"
    }
  | { type: "SHOW_MOCK_BANNER" }
  | { type: "HIDE_MOCK_BANNER" }

const emptyEntitlements = {
  warnings: [],
  features: {},
  has_license: false,
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
      entitlements: emptyEntitlements,
    },
    states: {
      idle: {
        on: {
          GET_ENTITLEMENTS: "gettingEntitlements",
          SHOW_MOCK_BANNER: { actions: "assignMockEntitlements" },
          HIDE_MOCK_BANNER: "gettingEntitlements",
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
      assignMockEntitlements: assign({
        entitlements: (_) => MockEntitlementsWithWarnings,
      }),
    },
    services: {
      getEntitlements: () => API.getEntitlements(),
    },
  },
)
