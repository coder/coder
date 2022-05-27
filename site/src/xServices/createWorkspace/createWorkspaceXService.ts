import { assign, createMachine } from "xstate"
import { createWorkspace, getTemplates, getTemplateVersionSchema } from "../../api/api"
import { CreateWorkspaceRequest, ParameterSchema, Template, Workspace } from "../../api/typesGenerated"

type CreateWorkspaceContext = {
  organizationId: string
  templates?: Template[]
  selectedTemplate?: Template
  templateSchema?: ParameterSchema[]
  createWorkspaceRequest?: CreateWorkspaceRequest
  createdWorkspace?: Workspace
  // This is useful when the user wants to create a workspace from the template
  // page having it pre selected. It is string or null because of the
  // useSearchQuery
  preSelectedTemplateName: string | null
}

type CreateWorkspaceEvent =
  | {
      type: "SELECT_TEMPLATE"
      template: Template
    }
  | {
      type: "CREATE_WORKSPACE"
      request: CreateWorkspaceRequest
    }

export const createWorkspaceMachine = createMachine(
  {
    id: "createWorkspaceState",
    initial: "gettingTemplates",
    schema: {
      context: {} as CreateWorkspaceContext,
      events: {} as CreateWorkspaceEvent,
      services: {} as {
        getTemplates: {
          data: Template[]
        }
        getTemplateSchema: {
          data: ParameterSchema[]
        }
        createWorkspace: {
          data: Workspace
        }
      },
    },
    tsTypes: {} as import("./createWorkspaceXService.typegen").Typegen0,
    on: {
      SELECT_TEMPLATE: {
        actions: ["assignSelectedTemplate"],
        target: "gettingTemplateSchema",
      },
    },
    states: {
      gettingTemplates: {
        invoke: {
          src: "getTemplates",
          onDone: [
            {
              actions: ["assignTemplates", "assignPreSelectedTemplate"],
              target: "gettingTemplateSchema",
              cond: "hasValidPreSelectedTemplate",
            },
            {
              actions: ["assignTemplates"],
              target: "selectingTemplate",
            },
          ],
          onError: {
            target: "error",
          },
        },
      },
      selectingTemplate: {
        on: {
          SELECT_TEMPLATE: {
            actions: ["assignSelectedTemplate"],
            target: "gettingTemplateSchema",
          },
        },
      },
      gettingTemplateSchema: {
        invoke: {
          src: "getTemplateSchema",
          onDone: {
            actions: ["assignTemplateSchema"],
            target: "fillingParams",
          },
          onError: {
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
        invoke: {
          src: "createWorkspace",
          onDone: {
            actions: ["onCreateWorkspace"],
            target: "created",
          },
          onError: {
            target: "error",
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
      createWorkspace: (context) => {
        const { createWorkspaceRequest, organizationId } = context

        if (!createWorkspaceRequest) {
          throw new Error("No create workspace request")
        }

        return createWorkspace(organizationId, createWorkspaceRequest)
      },
    },
    guards: {
      hasValidPreSelectedTemplate: (ctx, event) => {
        if (!ctx.preSelectedTemplateName) {
          return false
        }
        const template = event.data.find((template) => template.name === ctx.preSelectedTemplateName)
        return !!template
      },
    },
    actions: {
      assignTemplates: assign({
        templates: (_, event) => event.data,
      }),
      assignSelectedTemplate: assign({
        selectedTemplate: (_, event) => event.template,
      }),
      assignTemplateSchema: assign({
        // Only show parameters that are allowed to be overridden.
        // CLI code: https://github.com/coder/coder/blob/main/cli/create.go#L152-L155
        templateSchema: (_, event) => event.data.filter((param) => param.allow_override_source),
      }),
      assignCreateWorkspaceRequest: assign({
        createWorkspaceRequest: (_, event) => event.request,
      }),
      assignPreSelectedTemplate: assign({
        selectedTemplate: (ctx, event) => {
          const selectedTemplate = event.data.find((template) => template.name === ctx.preSelectedTemplateName)
          // The proper validation happens on hasValidPreSelectedTemplate
          if (!selectedTemplate) {
            throw new Error("Invalid template selected")
          }

          return selectedTemplate
        },
      }),
    },
  },
)
