import { getTemplateExamples } from "api/api";
import { TemplateExample } from "api/typesGenerated";
import { assign, createMachine } from "xstate";

export interface StarterTemplateContext {
  organizationId: string;
  exampleId: string;
  starterTemplate?: TemplateExample;
  error?: unknown;
}

export const starterTemplateMachine = createMachine(
  {
    id: "starterTemplate",
    predictableActionArguments: true,
    schema: {
      context: {} as StarterTemplateContext,
      services: {} as {
        loadStarterTemplate: {
          data: TemplateExample;
        };
      },
    },
    tsTypes: {} as import("./starterTemplateXService.typegen").Typegen0,
    initial: "loading",
    states: {
      loading: {
        invoke: {
          src: "loadStarterTemplate",
          onDone: {
            actions: ["assignStarterTemplate"],
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
      loadStarterTemplate: async ({ organizationId, exampleId }) => {
        const examples = await getTemplateExamples(organizationId);
        const starterTemplate = examples.find(
          (example) => example.id === exampleId,
        );
        if (!starterTemplate) {
          throw new Error(`Example ${exampleId} not found.`);
        }
        return starterTemplate;
      },
    },
    actions: {
      assignError: assign({
        error: (_, { data }) => data,
      }),
      assignStarterTemplate: assign({
        starterTemplate: (_, { data }) => data,
      }),
    },
  },
);
