import { Permissions } from "xServices/auth/authXService";
import { assign, createMachine } from "xstate";
import * as API from "../../api/api";
import * as TypesGen from "../../api/typesGenerated";

export interface TemplatesContext {
  organizationId: string;
  permissions: Permissions;
  templates?: TypesGen.Template[];
  examples?: TypesGen.TemplateExample[];
  error?: unknown;
}

export const templatesMachine = createMachine(
  {
    id: "templatesState",
    predictableActionArguments: true,
    tsTypes: {} as import("./templatesXService.typegen").Typegen0,
    schema: {
      context: {} as TemplatesContext,
      services: {} as {
        load: {
          data: {
            templates: TypesGen.Template[];
            examples: TypesGen.TemplateExample[];
          };
        };
      },
    },
    initial: "loading",
    states: {
      loading: {
        invoke: {
          src: "load",
          id: "load",
          onDone: {
            actions: ["assignData"],
            target: "idle",
          },
          onError: {
            actions: "assignError",
            target: "idle",
          },
        },
      },
      idle: {
        type: "final",
      },
    },
  },
  {
    actions: {
      assignData: assign({
        templates: (_, event) => event.data.templates,
        examples: (_, event) => event.data.examples,
      }),
      assignError: assign({
        error: (_, { data }) => data,
      }),
    },
    services: {
      load: async ({ organizationId, permissions }) => {
        const [templates, examples] = await Promise.all([
          API.getTemplates(organizationId),
          permissions.createTemplates
            ? API.getTemplateExamples(organizationId)
            : Promise.resolve([]),
        ]);

        return {
          templates,
          examples,
        };
      },
    },
  },
);
