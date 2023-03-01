import {
  getTemplateByName,
  getTemplateVersion,
  getTemplateVersionVariables,
} from "api/api"
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

  createTemplateVersionRequest?: CreateTemplateVersionRequest

  getTemplateError?: Error | unknown
  getActiveTemplateVersionError?: Error | unknown
  getTemplateVariablesError?: Error | unknown
  updateTemplateError?: Error | unknown
}

type UpdateTemplateEvent = {
  type: "UPDATE_TEMPLATE_EVENT"
  request: CreateTemplateVersionRequest // FIXME
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
        getActiveTemplateVersion: {
          data: TemplateVersion
        }
        getTemplateVariables: {
          data: TemplateVersionVariable[]
        }
        updateTemplate: {
          data: CreateTemplateVersionRequest
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
              target: "gettingActiveTemplateVersion",
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
        on: {
          UPDATE_TEMPLATE_EVENT: {
            actions: ["assignCreateTemplateVersionRequest"],
            target: "updatingTemplate",
          },
        },
      },
      updatingTemplate: {
        entry: "clearUpdateTemplateError",
        invoke: {
          src: "updateTemplate",
          onDone: {
            actions: ["onUpdateTemplate"],
            target: "updated",
          },
          onError: {
            actions: ["assignUpdateTemplateError"],
            target: "fillingParams",
          },
        },
        tags: ["submitting"],
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
      updateTemplate: (context) => {
        console.log(context.createTemplateVersionRequest)
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
      assignCreateTemplateVersionRequest: assign({
        createTemplateVersionRequest: (_, event) => event.request,
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
      assignGetActiveTemplateVersionError: assign({
        getActiveTemplateVersionError: (_, event) => event.data,
      }),
      clearGetActiveTemplateVersionError: assign({
        getActiveTemplateVersionError: (_) => undefined,
      }),
      assignUpdateTemplateError: assign({
        updateTemplateError: (_, event) => event.data,
      }),
      clearUpdateTemplateError: assign({
        updateTemplateError: (_) => undefined,
      }),
    },
  },
)
