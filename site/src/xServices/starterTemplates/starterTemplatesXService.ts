import { getTemplateExamples } from "api/api"
import { TemplateExample } from "api/typesGenerated"
import { assign, createMachine } from "xstate"

export interface StarterTemplatesContext {
  organizationId: string
  starterTemplates?: TemplateExample[]
  error?: unknown
}

export const starterTemplatesMachine = createMachine(
  {
    id: "starterTemplates",
    predictableActionArguments: true,
    schema: {
      context: {} as StarterTemplatesContext,
      services: {} as {
        loadStarterTemplates: {
          data: TemplateExample[]
        }
      },
    },
    tsTypes: {} as import("./starterTemplatesXService.typegen").Typegen0,
    initial: "loading",
    states: {
      loading: {
        invoke: {
          src: "loadStarterTemplates",
          onDone: {
            actions: ["assignStarterTemplates"],
            target: "idle.ok",
          },
          onError: {
            actions: ["assignError"],
            target: "idle.error",
          },
        },
      },
      idle: {
        initial: "ok",
        states: {
          ok: { type: "final" },
          error: { type: "final" },
        },
      },
    },
  },
  {
    services: {
      loadStarterTemplates: ({ organizationId }) =>
        getTemplateExamples(organizationId),
    },
    actions: {
      assignError: assign({
        error: (_, { data }) => data,
      }),
      assignStarterTemplates: assign({
        starterTemplates: (_, { data }) => data,
      }),
    },
  },
)
