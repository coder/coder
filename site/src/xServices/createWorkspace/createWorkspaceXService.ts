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
    states: {
      gettingTemplates: {
        invoke: {
          src: "getTemplates",
          onDone: {
            actions: ["assignTemplates"],
            target: "selectingTemplate",
          },
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
            target: "fillingForm",
          },
          onError: {
            target: "error",
          },
        },
      },
      fillingForm: {
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
    actions: {
      assignTemplates: assign({
        templates: (_, event) => event.data,
      }),
      assignSelectedTemplate: assign({
        selectedTemplate: (_, event) => event.template,
      }),
      assignTemplateSchema: assign({
        templateSchema: (_, event) => event.data,
      }),
      assignCreateWorkspaceRequest: assign({
        createWorkspaceRequest: (_, event) => event.request,
      }),
    },
  },
)
