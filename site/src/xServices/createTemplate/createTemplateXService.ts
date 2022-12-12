import { getTemplateExamples } from "api/api"
import { Template, TemplateExample } from "api/typesGenerated"
import { MockTemplate } from "testHelpers/entities"
import { assign, createMachine } from "xstate"

interface CreateTemplateContext {
  organizationId: string
  // It can be null because it is being passed from query string
  exampleId?: string | null
  error?: unknown
  starterTemplate?: TemplateExample
}

export const createTemplateMachine = createMachine(
  {
    id: "createTemplate",
    predictableActionArguments: true,
    schema: {
      context: {} as CreateTemplateContext,
      events: {} as { type: "CREATE"; data: any },
      services: {} as {
        loadingStarterTemplate: {
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
            actions: ["assignError"],
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
      createTemplate: async () => {
        console.log("CALL CREATE TEMPLATE!")
        return MockTemplate
      },
    },
    actions: {
      assignError: (_, { data }) => assign({ error: data }),
      assignStarterTemplate: (_, { data }) => assign({ starterTemplate: data }),
    },
    guards: {
      isExampleProvided: ({ exampleId }) => Boolean(exampleId),
    },
  },
)
