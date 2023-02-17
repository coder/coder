import { getTemplateByName, getTemplateVersionVariables } from "api/api"
import {
  CreateWorkspaceBuildRequest,
  Template,
  TemplateVersionVariable,
} from "api/typesGenerated"
import { assign, createMachine } from "xstate"

type TemplateVariablesContext = {
  organizationId: string
  templateName: string

  selectedTemplate?: Template
  templateVariables?: TemplateVersionVariable[]

  getTemplateError?: Error | unknown
  getTemplateVariablesError?: Error | unknown
}

type UpdateTemplateEvent = {
  type: "UPDATE_TEMPLATE"
  request: CreateWorkspaceBuildRequest // FIXME
}

export const templateVariablesMachine = createMachine(
  {
    id: "templateVariablesState",
    predictableActionArguments: true,
    tsTypes: {} as import("./templateVariablesXService.typegen").Typegen0,
    schema: {
      context: {} as TemplateVariablesContext,
      events: {} as UpdateTemplateEvent,
      services: {} as {
        getTemplate: {
          data: Template
        }
        getTemplateVariables: {
          data: TemplateVersionVariable[]
        }
      },
    },
    initial: "gettingTemplate",
    states: {
      gettingTemplate: {
        entry: "clearGetTemplateError",
        invoke: {
          src: "getTemplate",
          onDone: [
            {
              actions: ["assignTemplate"],
              target: "gettingTemplateVariables",
            },
          ],
          onError: {
            actions: ["assignGetTemplateError"],
            target: "error",
          },
        },
      },
      gettingTemplateVariables: {
        entry: "clearGetTemplateVariablesError",
        invoke: {
          src: "getTemplateVariables",
          onDone: [
            {
              actions: ["assignTemplateVariables"],
              target: "fillingParams",
            },
          ],
          onError: {
            actions: ["assignGetTemplateVariablesError"],
            target: "error",
          },
        },
      },
      fillingParams: {
        // FIXME on
      },
      updated: {
        entry: "onUpdateTemplate",
        type: "final",
      },
      error: {},
    },
  },
  {
    services: {
      getTemplate: (context) => {
        const { organizationId, templateName } = context
        return getTemplateByName(organizationId, templateName)
      },
      getTemplateVariables: (context) => {
        const { selectedTemplate } = context

        if (!selectedTemplate) {
          throw new Error("No template selected")
        }

        return getTemplateVersionVariables(selectedTemplate.active_version_id)
      },
    },
    actions: {
      assignTemplate: assign({
        selectedTemplate: (_, event) => event.data,
      }),
      assignTemplateVariables: assign({
        templateVariables: (_, event) => event.data,
      }),
      assignGetTemplateError: assign({
        getTemplateError: (_, event) => event.data,
      }),
      clearGetTemplateError: assign({
        getTemplateError: (_) => undefined,
      }),
      assignGetTemplateVariablesError: assign({
        getTemplateVariablesError: (_, event) => event.data,
      }),
      clearGetTemplateVariablesError: assign({
        getTemplateVariablesError: (_) => undefined,
      }),
    },
  },
)
