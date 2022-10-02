import { assign, createMachine } from "xstate"
import {
  createWorkspace,
  getTemplates,
  getTemplateVersionParameters,
  getTemplateVersionSchema,
} from "../../api/api"
import {
  CreateWorkspaceRequest,
  DeprecatedParameterSchema,
  Template,
  TemplateVersionParameter,
  Workspace,
} from "../../api/typesGenerated"

type CreateWorkspaceContext = {
  organizationId: string
  templateName: string
  templates?: Template[]
  selectedTemplate?: Template
  templateSchema?: DeprecatedParameterSchema[]
  templateVersionParameters?: TemplateVersionParameter[]
  templateVersionParametersError?: Error | unknown
  createWorkspaceRequest?: CreateWorkspaceRequest
  createdWorkspace?: Workspace
  createWorkspaceError?: Error | unknown
  getTemplatesError?: Error | unknown
  getTemplateSchemaError?: Error | unknown
}

type CreateWorkspaceEvent = {
  type: "CREATE_WORKSPACE"
  request: CreateWorkspaceRequest
}

export const createWorkspaceMachine = createMachine(
  {
    id: "createWorkspaceState",
    predictableActionArguments: true,
    tsTypes: {} as import("./createWorkspaceXService.typegen").Typegen0,
    schema: {
      context: {} as CreateWorkspaceContext,
      events: {} as CreateWorkspaceEvent,
      services: {} as {
        getTemplates: {
          data: Template[]
        }
        getTemplateSchema: {
          data: DeprecatedParameterSchema[]
        }
        getTemplateVersionParameters: {
          data: TemplateVersionParameter[]
        }
        createWorkspace: {
          data: Workspace
        }
      },
    },
    initial: "gettingTemplates",
    states: {
      gettingTemplates: {
        entry: "clearGetTemplatesError",
        invoke: {
          src: "getTemplates",
          onDone: [
            {
              actions: ["assignTemplates"],
              cond: "areTemplatesEmpty",
            },
            {
              actions: ["assignTemplates", "assignSelectedTemplate"],
              target: "gettingTemplateSchema",
            },
          ],
          onError: {
            actions: ["assignGetTemplatesError"],
            target: "error",
          },
        },
      },
      gettingTemplateSchema: {
        entry: "clearGetTemplateSchemaError",
        invoke: {
          src: "getTemplateSchema",
          onDone: {
            actions: ["assignTemplateSchema"],
            target: "gettingTemplateVersionParameters",
          },
          onError: {
            actions: ["assignGetTemplateSchemaError"],
            target: "error",
          },
        },
      },
      gettingTemplateVersionParameters: {
        entry: "clearTemplateVersionParametersError",
        invoke: {
          src: "getTemplateVersionParameters",
          onDone: {
            actions: ["assignTemplateVersionParameters"],
            target: "fillingParams",
          },
          onError: {
            actions: ["assignTemplateVersionParametersError"],
            target: "error",
          },
        },
      },
      fillingParams: {
        on: {
          CREATE_WORKSPACE: {
            actions: ["assignCreateWorkspaceRequest"],
            target: "creatingWorkspace",
          },
        },
      },
      creatingWorkspace: {
        entry: "clearCreateWorkspaceError",
        invoke: {
          src: "createWorkspace",
          onDone: {
            actions: ["onCreateWorkspace"],
            target: "created",
          },
          onError: {
            actions: ["assignCreateWorkspaceError"],
            target: "fillingParams",
          },
        },
      },
      created: {
        type: "final",
      },
      error: {},
    },
  },
  {
    services: {
      getTemplates: (context) => getTemplates(context.organizationId),
      getTemplateSchema: (context) => {
        const { selectedTemplate } = context

        if (!selectedTemplate) {
          throw new Error("No selected template")
        }

        return getTemplateVersionSchema(selectedTemplate.active_version_id)
      },
      getTemplateVersionParameters: (context) => {
        const { selectedTemplate } = context

        if (!selectedTemplate) {
          throw new Error("No selected template")
        }

        return getTemplateVersionParameters(selectedTemplate.active_version_id)
      },
      createWorkspace: (context) => {
        const { createWorkspaceRequest, organizationId } = context

        if (!createWorkspaceRequest) {
          throw new Error("No create workspace request")
        }

        return createWorkspace(organizationId, "me", createWorkspaceRequest)
      },
    },
    guards: {
      areTemplatesEmpty: (_, event) => event.data.length === 0,
    },
    actions: {
      assignTemplates: assign({
        templates: (_, event) => event.data,
      }),
      assignSelectedTemplate: assign({
        selectedTemplate: (ctx, event) => {
          const templates = event.data.filter((template) => template.name === ctx.templateName)
          return templates.length > 0 ? templates[0] : undefined
        },
      }),
      assignTemplateSchema: assign({
        // Only show parameters that are allowed to be overridden.
        // CLI code: https://github.com/coder/coder/blob/main/cli/create.go#L152-L155
        templateSchema: (_, event) => event.data.filter((param) => param.allow_override_source),
      }),
      assignTemplateVersionParameters: assign({
        templateVersionParameters: (_, event) => event.data,
      }),
      assignTemplateVersionParametersError: assign({
        templateVersionParametersError: (_, event) => event.data,
      }),
      clearTemplateVersionParametersError: assign({
        templateVersionParametersError: (_) => undefined,
      }),
      assignCreateWorkspaceRequest: assign({
        createWorkspaceRequest: (_, event) => event.request,
      }),
      assignCreateWorkspaceError: assign({
        createWorkspaceError: (_, event) => event.data,
      }),
      clearCreateWorkspaceError: assign({
        createWorkspaceError: (_) => undefined,
      }),
      assignGetTemplatesError: assign({
        getTemplatesError: (_, event) => event.data,
      }),
      clearGetTemplatesError: assign({
        getTemplatesError: (_) => undefined,
      }),
      assignGetTemplateSchemaError: assign({
        getTemplateSchemaError: (_, event) => event.data,
      }),
      clearGetTemplateSchemaError: assign({
        getTemplateSchemaError: (_) => undefined,
      }),
    },
  },
)
