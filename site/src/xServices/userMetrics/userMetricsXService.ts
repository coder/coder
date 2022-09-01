import { DAUsResponse } from "api/typesGenerated"
import { assign, createMachine } from "xstate"
import * as API from "../../api/api"

export interface UserMetricsContext {
  userMetricsData: DAUsResponse
}

export const userMetricsMachine = createMachine(
  {
    id: "userMetrics",
    schema: {
      context: {} as UserMetricsContext,
      services: {} as {
        loadMetrics: {
          data: DAUsResponse
        }
      },
    },
    tsTypes: {} as import("./userMetricsXService.typegen").Typegen0,
    initial: "loadingMetrics",
    states: {
      loadingMetrics: {
        invoke: {
          src: "loadMetrics",
          onDone: {
            target: "success",
            actions: ["assignDataMetrics"],
          },
        },
      },
      success: {
        type: "final",
      },
    },
  },
  {
    actions: {
      assignDataMetrics: assign({
        userMetricsData: (_, event) => event.data,
      }),
    },
    services: {
      loadMetrics: () => API.getDAUs,
    },
  },
)
