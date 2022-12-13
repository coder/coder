import { createValidTemplate, getTemplateExamples } from "api/api"
import { Template, TemplateExample } from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"

interface CreateTemplateContext {
  organizationId: string
  // It can be null because it is being passed from query string
  exampleId?: string | null
  error?: unknown
  starterTemplate?: TemplateExample
}

export interface CreateTemplateData {
  name: string
  display_name: string
  description: string
  icon: string
  default_ttl_hours: number
  allow_user_cancel_workspace_jobs: boolean
}

export const createTemplateMachine = createMachine(
  {
    id: "createTemplate",
    predictableActionArguments: true,
    schema: {
      context: {} as CreateTemplateContext,
      events: {} as { type: "CREATE"; data: CreateTemplateData },
      services: {} as {
        loadStarterTemplate: {
          data: TemplateExample
        }
        createTemplate: {
          data: Template
        }
      },
    },
    tsTypes: {} as import("./createTemplateXService.typegen").Typegen0,
    initial: "starting",
    states: {
      starting: {
        always: [
          { target: "loadingStarterTemplate", cond: "isExampleProvided" },
          { target: "idle" },
        ],
        tags: ["loading"],
      },
      loadingStarterTemplate: {
        invoke: {
          src: "loadStarterTemplate",
          onDone: {
            target: "idle",
            actions: ["assignStarterTemplate"],
          },
          onError: {
            target: "idle",
            actions: ["assignError"],
          },
        },
        tags: ["loading"],
      },
      idle: {
        on: {
          CREATE: "creating",
        },
      },
      creating: {
        invoke: {
          src: "createTemplate",
          onDone: {
            target: "created",
            actions: ["onCreate"],
          },
          onError: {
            target: "idle",
            actions: ["displayError"],
          },
        },
      },
      created: {
        type: "final",
      },
    },
  },
  {
    services: {
      loadStarterTemplate: async ({ organizationId, exampleId }) => {
        if (!exampleId) {
          throw new Error(`Example ID is not defined.`)
        }
        const examples = await getTemplateExamples(organizationId)
        const starterTemplate = examples.find(
          (example) => example.id === exampleId,
        )
        if (!starterTemplate) {
          throw new Error(`Example ${exampleId} not found.`)
        }
        return starterTemplate
      },
      createTemplate: async ({ organizationId, exampleId }, { data }) => {
        if (!exampleId) {
          throw new Error("Example ID not provided")
        }
        return createValidTemplate(organizationId, exampleId, {
          ...data,
          // hours to milliseconds
          default_ttl_ms: data.default_ttl_hours * 60 * 60 * 100,
        })
      },
    },
    actions: {
      assignError: assign({ error: (_, { data }) => data }),
      assignStarterTemplate: assign({ starterTemplate: (_, { data }) => data }),
      displayError: (_, { data }) => {
        if (data instanceof Error) {
          displayError(data.message)
          return
        }
        console.warn(`data is not an Error`)
      },
    },
    guards: {
      isExampleProvided: ({ exampleId }) => Boolean(exampleId),
    },
  },
)
