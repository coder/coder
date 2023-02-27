import { getTemplateByName, getTemplateVersion, getTemplateVersionVariables } from "api/api"
import {
  CreateTemplateVersionRequest,
  Template,
  TemplateVersion,
  TemplateVersionVariable,
} from "api/typesGenerated"
import { assign, createMachine } from "xstate"

type TemplateVariablesContext = {
  organizationId: string
  templateName: string

  template?: Template
  activeTemplateVersion?: TemplateVersion
  templateVariables?: TemplateVersionVariable[]

  getTemplateError?: Error | unknown
  getActiveTemplateVersionError?: Error | unknown
  getTemplateVariablesError?: Error | unknown
}

type CreateTemplateVersionEvent = {
  type: "CREATE_TEMPLATE_VERSION"
  request: CreateTemplateVersionRequest // FIXME
}

export const templateVariablesMachine = createMachine(
  {
    id: "templateVariablesState",
    predictableActionArguments: true,
    tsTypes: {} as import("./templateVariablesXService.typegen").Typegen0,
    schema: {
      context: {} as TemplateVariablesContext,
      events: {} as CreateTemplateVersionEvent,
      services: {} as {
        getTemplate: {
          data: Template
        }
        getActiveTemplateVersion: {
          data: TemplateVersion
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
      gettingActiveTemplateVersion: {
        entry: "clearGetActiveTemplateVersionError",
        invoke: {
          src: "getActiveTemplateVersion",
          onDone: [
            {
              actions: ["assignActiveTemplateVersion"],
              target: "gettingTemplateVariables",
            },
          ],
          onError: {
            actions: ["assignGetActiveTemplateVersionError"],
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
      getActiveTemplateVersion: (context) => {
        const { template } = context
        if (!template) {
          throw new Error("No template selected")
        }
        return getTemplateVersion(template.active_version_id)

      },
      getTemplateVariables: (context) => {
        const { template } = context
        if (!template) {
          throw new Error("No template selected")
        }
        return getTemplateVersionVariables(template.active_version_id)
      },
    },
    actions: {
      assignTemplate: assign({
        template: (_, event) => event.data,
      }),
      assignActiveTemplateVersion: assign({
        activeTemplateVersion: (_, event) => event.data,
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
