import { ActorRefFrom, createMachine, sendParent, assign } from "xstate"

export interface PaginationContext {
  page: number
  limit: number
  updateURL: (page: number) => void
}

export type PaginationEvent =
  | { type: "NEXT_PAGE" }
  | { type: "PREVIOUS_PAGE" }
  | { type: "GO_TO_PAGE"; page: number }
  | { type: "RESET_PAGE" }

export type PaginationMachineRef = ActorRefFrom<typeof paginationMachine>

export const paginationMachine = createMachine(
  {
    id: "paginationMachine",
    predictableActionArguments: true,
    tsTypes: {} as import("./paginationXService.typegen").Typegen0,
    schema: {
      context: {} as PaginationContext,
      events: {} as PaginationEvent,
    },
    initial: "idle",
    on: {
      NEXT_PAGE: {
        actions: ["assignNextPage", "updateURL", "sendRefreshData"],
      },
      PREVIOUS_PAGE: {
        actions: ["assignPreviousPage", "updateURL", "sendRefreshData"],
      },
      GO_TO_PAGE: {
        actions: ["assignPage", "updateURL", "sendRefreshData"],
      },
      RESET_PAGE: {
        actions: ["resetPage", "updateURL", "sendRefreshData"],
      },
    },
    states: {
      idle: {},
    },
  },
  {
    actions: {
      sendRefreshData: (_) => sendParent("REFRESH_DATA"),
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
      updateURL: (context) => {
        context.updateURL(context.page)
      },
    },
  },
)
