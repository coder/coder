import {
  checkAuthorization,
  createWorkspace,
  getTemplates,
  getTemplateVersionRichParameters,
  getTemplateVersionSchema,
} from "api/api"
import {
  CreateWorkspaceRequest,
  ParameterSchema,
  Template,
  TemplateVersionParameter,
  User,
  Workspace,
} from "api/typesGenerated"
import { assign, createMachine } from "xstate"

type CreateWorkspaceContext = {
  organizationId: string
  owner: User | null
  templateName: string
  templates?: Template[]
  selectedTemplate?: Template
  templateParameters?: TemplateVersionParameter[]
  templateSchema?: ParameterSchema[]
  createWorkspaceRequest?: CreateWorkspaceRequest
  createdWorkspace?: Workspace
  createWorkspaceError?: Error | unknown
  getTemplatesError?: Error | unknown
  getTemplateParametersError?: Error | unknown
  getTemplateSchemaError?: Error | unknown
  permissions?: Record<string, boolean>
  checkPermissionsError?: Error | unknown
}

type CreateWorkspaceEvent = {
  type: "CREATE_WORKSPACE"
  request: CreateWorkspaceRequest
  owner: User | null
}

type SelectOwnerEvent = {
  type: "SELECT_OWNER"
  owner: User | null
}

export const createWorkspaceMachine = createMachine(
  {
    id: "createWorkspaceState",
    predictableActionArguments: true,
    tsTypes: {} as import("./createWorkspaceXService.typegen").Typegen0,
    schema: {
      context: {} as CreateWorkspaceContext,
      events: {} as CreateWorkspaceEvent | SelectOwnerEvent,
      services: {} as {
        getTemplates: {
          data: Template[]
        }
        getTemplateParameters: {
          data: TemplateVersionParameter[]
        }
        getTemplateSchema: {
          data: ParameterSchema[]
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
            target: "gettingTemplateParameters",
          },
          onError: {
            actions: ["assignGetTemplateSchemaError"],
            target: "error",
          },
        },
      },
      gettingTemplateParameters: {
        entry: "clearGetTemplateParametersError",
        invoke: {
          src: "getTemplateParameters",
          onDone: {
            actions: ["assignTemplateParameters"],
            target: "checkingPermissions",
          },
          onError: {
            actions: ["assignGetTemplateParametersError"],
            target: "error",
          },
        },
      },
      checkingPermissions: {
        entry: "clearCheckPermissionsError",
        invoke: {
          src: "checkPermissions",
          id: "checkPermissions",
          onDone: {
            actions: "assignPermissions",
            target: "fillingParams",
          },
          onError: {
            actions: ["assignCheckPermissionsError"],
          },
        },
      },
      fillingParams: {
        on: {
          CREATE_WORKSPACE: {
            actions: ["assignCreateWorkspaceRequest", "assignOwner"],
            target: "creatingWorkspace",
          },
          SELECT_OWNER: {
            actions: ["assignOwner"],
            target: ["fillingParams"],
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
      getTemplateParameters: (context) => {
        const { selectedTemplate } = context

        if (!selectedTemplate) {
          throw new Error("No selected template")
        }

        return getTemplateVersionRichParameters(
          selectedTemplate.active_version_id,
        )
      },
      getTemplateSchema: (context) => {
        const { selectedTemplate } = context

        if (!selectedTemplate) {
          throw new Error("No selected template")
        }

        return getTemplateVersionSchema(selectedTemplate.active_version_id)
      },
      checkPermissions: async (context) => {
        if (!context.organizationId) {
          throw new Error("No organization ID")
        }

        // HACK: below, we pass in * for the owner_id, which is a hacky way of checking if the
        // current user can create a workspace on behalf of anyone within the org (only org owners should be able to do this).
        // This pattern should not be replicated outside of this narrow use case.
        const permissionsToCheck = {
          createWorkspaceForUser: {
            object: {
              resource_type: "workspace",
              organization_id: `${context.organizationId}`,
              owner_id: "*",
            },
            action: "create",
          },
        }

        return checkAuthorization({
          checks: permissionsToCheck,
        })
      },
      createWorkspace: (context) => {
        const { createWorkspaceRequest, organizationId, owner } = context

        if (!createWorkspaceRequest) {
          throw new Error("No create workspace request")
        }

        return createWorkspace(
          organizationId,
          owner?.id ?? "me",
          createWorkspaceRequest,
        )
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
          const templates = event.data.filter(
            (template) => template.name === ctx.templateName,
          )
          return templates.length > 0 ? templates[0] : undefined
        },
      }),
      assignTemplateParameters: assign({
        templateParameters: (_, event) => event.data,
      }),
      assignTemplateSchema: assign({
        // Only show parameters that are allowed to be overridden.
        // CLI code: https://github.com/coder/coder/blob/main/cli/create.go#L152-L155
        templateSchema: (_, event) => event.data,
      }),
      assignPermissions: assign({
        permissions: (_, event) => event.data as Record<string, boolean>,
      }),
      assignCheckPermissionsError: assign({
        checkPermissionsError: (_, event) => event.data,
      }),
      clearCheckPermissionsError: assign({
        checkPermissionsError: (_) => undefined,
      }),
      assignCreateWorkspaceRequest: assign({
        createWorkspaceRequest: (_, event) => event.request,
      }),
      assignOwner: assign({
        owner: (_, event) => event.owner,
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
      assignGetTemplateParametersError: assign({
        getTemplateParametersError: (_, event) => event.data,
      }),
      clearGetTemplateParametersError: assign({
        getTemplateParametersError: (_) => undefined,
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
