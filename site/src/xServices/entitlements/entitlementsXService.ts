import { assign, createMachine } from "xstate";
import * as API from "../../api/api";
import { Entitlements } from "../../api/typesGenerated";

export type EntitlementsContext = {
  entitlements?: Entitlements;
  getEntitlementsError?: unknown;
};

export const entitlementsMachine = createMachine(
  {
    /** @xstate-layout N4IgpgJg5mDOIC5RgHYBcCWaA2YC2qasAsgIYDGAFhimAHQBOYAZk7JQMQQD2tdNAN24Brek1ZxKAUXRZcBdLADaABgC6iUAAdusLBl6aQAD0QAWAGwAOOgGYAjACYArABoQAT0SOA7AE46C0czP2cVHzNIlRV7HwBfOPdCOXxCEgpqPnE2TjAGBm4GOi1sUjRmQrxGFhyZTBxUxVUNJBAdPUxDVtMESxsHF3cvBFtbAKcQsIiomPjE8FkGhSIyKhp6GDRMFCg6lOXYLl56QRENsDQ9pbTmo3b9LtAe2wtnOmcBi1Hos0c-Hx8Q0QVnsdBCfj8Vh8UKsYVscySi3kaVWmXOWxouyRjSIHDyBSKJTKFQYVU2V2RTXUd10DxQRmer3en2+Kl+-0BnkQsTM7yCZimkTM0ViCUR9UpKwy634EFwHAASlIAGJKgDKAAlbq17p16d1uZCgb1HBZ3n54fZIrCgrZHGKFhKcek1nx8YVFSr1VrqTraXqjMN7KE6CoLT4rWYbY4HJyev5QabnBYzA4LX5XmYEvMUNwIHAjMlropUesaR0DPqnogALRR401s3RaKORwqWyw2Ehe3zIuSl1o6oSdjlukMxDwlR0HwWAFsiz-PwqZz-Y0r3m2AXhaGhPxmHzOB1952lvibbZYp0HUcBg0m42wuwp5wfeHP9s98X7FHSvgYOVgDelbjggKjGiENgrpa1rJjGn6Ot+Ja-vQsAAK7kOQcDwH6FaPCYiBQXQSYpmmYyZsa9gqI4oYDM49gWMGPimrOR7Ygcp70O6DBAXhPSEcRqbBmRzhmBRIZhtBUawbG2ZxEAA */
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
      refresh: {
        invoke: {
          id: "refreshEntitlements",
          src: "refreshEntitlements",
          onDone: {
            target: "gettingEntitlements",
          },
          onError: {
            target: "error",
            actions: ["assignGetEntitlementsError"],
          },
        },
        entry: "clearGetEntitlementsError",
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
            target: "error",
            actions: ["assignGetEntitlementsError"],
          },
        },
      },
      idle: {
        on: {
          REFRESH: "refresh",
        },
      },
      success: {
        type: "final",
      },
      error: {
        on: {
          REFRESH: "refresh",
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
        getEntitlementsError: (_, event) => {
          return event.data;
        },
      }),
      clearGetEntitlementsError: assign({
        getEntitlementsError: (_) => undefined,
      }),
    },
    services: {
      refreshEntitlements: async () => {
        return API.refreshEntitlements();
      },
      getEntitlements: async () => {
        // Entitlements is injected by the Coder server into the HTML document.
        const entitlements = document.querySelector(
          "meta[property=entitlements]",
        );
        if (entitlements) {
          const rawContent = entitlements.getAttribute("content");
          try {
            return JSON.parse(rawContent as string);
          } catch (ex) {
            // Ignore this and fetch as normal!
          }
        }

        return API.getEntitlements();
      },
    },
  },
);
