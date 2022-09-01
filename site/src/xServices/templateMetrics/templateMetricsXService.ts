import { TemplateDAUsResponse } from "api/typesGenerated"
import { assign, createMachine } from "xstate"
import * as API from "../../api/api"

export interface TemplateMetricsContext {
  templateId: string
  templateMetricsData: TemplateDAUsResponse
}

export const templateMetricsMachine = createMachine(
  {
    id: "templateMetrics",
    schema: {
      context: {} as TemplateMetricsContext,
      services: {} as {
        loadMetrics: {
          data: TemplateDAUsResponse
        }
      },
    },
    tsTypes: {} as import("./templateMetricsXService.typegen").Typegen0,
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
        templateMetricsData: (_, event) => event.data,
      }),
    },
    services: {
      loadMetrics: (context) => API.getTemplateDAUs(context.templateId),
    },
  },
)
