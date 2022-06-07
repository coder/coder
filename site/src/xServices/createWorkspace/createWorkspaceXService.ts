import { assign, createMachine } from "xstate"
import { createWorkspace, getTemplates, getTemplateVersionSchema } from "../../api/api"
import { CreateWorkspaceRequest, ParameterSchema, Template, Workspace } from "../../api/typesGenerated"

type CreateWorkspaceContext = {
  organizationId: string
  templateName: string
  templates?: Template[]
  selectedTemplate?: Template
  templateSchema?: ParameterSchema[]
  createWorkspaceRequest?: CreateWorkspaceRequest
  createdWorkspace?: Workspace
}

type CreateWorkspaceEvent =
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
    states: {
      gettingTemplates: {
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
            target: "error",
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
      areTemplatesEmpty: (_, event) => event.data.length === 0,
    },
    actions: {
      assignTemplates: assign({
        templates: (_, event) => event.data,
      }),
      assignSelectedTemplate: assign({
        selectedTemplate: (ctx, event) => {
          for (const template of event.data) {
            if (template.name === ctx.templateName) {
              return template
            }
          }
        },
      }),
      assignTemplateSchema: assign({
        // Only show parameters that are allowed to be overridden.
        // CLI code: https://github.com/coder/coder/blob/main/cli/create.go#L152-L155
        templateSchema: (_, event) => event.data.filter((param) => param.allow_override_source),
      }),
      assignCreateWorkspaceRequest: assign({
        createWorkspaceRequest: (_, event) => event.request,
      }),
    },
  },
)
