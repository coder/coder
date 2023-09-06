import { ActorRefFrom, createMachine, sendParent, assign } from "xstate";

export interface PaginationContext {
  page: number;
  limit: number;
}

export type PaginationEvent =
  | { type: "NEXT_PAGE" }
  | { type: "PREVIOUS_PAGE" }
  | { type: "GO_TO_PAGE"; page: number }
  | { type: "RESET_PAGE" };

export type PaginationMachineRef = ActorRefFrom<typeof paginationMachine>;

export const paginationMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QAcCGUCWA7VAXDA9lgLKoDGAFtmAMQByAogBoAqA+gAoCCA4gwNoAGALqIUBWBnxExIAB6IATADZBAOgAcGgCwBWDYMEHBAdm2KANCACeiEwEZ7axboCcbkw8-3XGgL5+VmiYONIk5FRYtBwASgwAagCSAPIAqgDKnLwCIrLIElKEWLIKCAC09hrKarq6AMwarsraGormuiYaVrYIDk4u7q7e3r4BQejYeEWklNQ0PMlsLIvcfEKiSCD5kmElduq69vrKyiaCjqfu3YgauupatYp1eia+o4FbE6HTEXNx6Qx2KschtxDsintyvZqlVzmdGs0tIpbtcENplIoanV7M8jrpFCZntoAh8sAQIHA8l8pkQZpEwGoAE5gVAQHpgwoyTalZSuNTuV6CFzabSCFqdVFmdTPMU+dHnZrKMafEI08KzKJ5Aq7blKJwC1xC3QisUaCU2RDKQ5qGXaOWqaHokl+IA */
  createMachine(
    {
      tsTypes: {} as import("./paginationXService.typegen").Typegen0,
      schema: {
        context: {} as PaginationContext,
        events: {} as PaginationEvent,
      },
      predictableActionArguments: true,
      id: "paginationMachine",
      initial: "ready",
      on: {
        NEXT_PAGE: {
          actions: ["assignNextPage", "sendUpdatePage"],
        },
        PREVIOUS_PAGE: {
          actions: ["assignPreviousPage", "sendUpdatePage"],
        },
        GO_TO_PAGE: {
          actions: ["assignPage", "sendUpdatePage"],
        },
        RESET_PAGE: {
          actions: ["resetPage", "sendUpdatePage"],
        },
      },
      states: {
        ready: {},
      },
    },
    {
      actions: {
        sendUpdatePage: sendParent((context) => ({
          type: "UPDATE_PAGE",
          page: context.page.toString(),
        })),
        assignNextPage: assign({
          page: (context) => context.page + 1,
        }),
        assignPreviousPage: assign({
          page: (context) => context.page - 1,
        }),
        assignPage: assign({
          page: (_, event) => event.page,
        }),
        resetPage: assign({
          page: (_) => 1,
        }),
      },
    },
  );
