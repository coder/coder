import {
  checkAuthorization,
  createWorkspace,
  getTemplates,
  getTemplateVersionGitAuth,
  getTemplateVersionRichParameters,
  getTemplateVersionSchema,
} from "api/api"
import {
  CreateWorkspaceRequest,
  ParameterSchema,
  Template,
  TemplateVersionGitAuth,
  TemplateVersionParameter,
  User,
  Workspace,
} from "api/typesGenerated"
import { assign, createMachine } from "xstate"

export const REFRESH_GITAUTH_BROADCAST_CHANNEL = "gitauth_refresh"

type CreateWorkspaceContext = {
  organizationId: string
  owner: User | null
  templateName: string
  templates?: Template[]
  selectedTemplate?: Template
  templateParameters?: TemplateVersionParameter[]
  templateSchema?: ParameterSchema[]
  templateGitAuth?: TemplateVersionGitAuth[]
  createWorkspaceRequest?: CreateWorkspaceRequest
  createdWorkspace?: Workspace
  createWorkspaceError?: Error | unknown
  getTemplatesError?: Error | unknown
  getTemplateParametersError?: Error | unknown
  getTemplateGitAuthError?: Error | unknown
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

type RefreshGitAuthEvent = {
  type: "REFRESH_GITAUTH"
}

export const createWorkspaceMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QGMBOYCGAXMB1A9qgNawAOGyYAyltmAHQxZYCWAdlACpgC2pANnVgBiCPjYN2AN3xEGaTDgLEyFarRyMwzdl14ChCafmTYW4gNoAGALrWbiUKXywWrcY5AAPRABYATAA0IACeiACMvgCs9ADssQBsVgDM-v5JyQnJUQkAvrnBCnTKJOSUNHRaOhzcfII4ImIS9MZy9EVKhKVqFZpMrDX69XBGbDKm7mz2FuEOSCDOrpOePgjJAJzr9AAcVr4J677bCeFWB3vBYQgBvvQJB-5R6+GxVulPUfmF6MVdquUaBj9XS1AwNYRgVCoQj0MEAM0IPHaP06KjK6kqwMGdUMxgm5imtnsnkWbgJK0QGy2u32h2Op3OvkuiH8vis9HCDxO0XC2z5CX8XxAHTwf3RvSB2gGehxOCoyAAFrwMKJxJIxrJ5CjRWieoCqtLQcN5UqeBhRuMzJYibYSS4yR55qtTv52bFeYlfPEor4XuFmddwlscqz1mdfMlfZlfEKRSV-hi+lKQUM6CblRCoTD4YjkYodd0AZjk9iwdRFcqLSYrYS7Lb5qTlk6Im83R6El7Yj6-QH4gl6M8ctsoht7skrJ8CsLtfHxfqsTKywAFDCoDA8bSQxpqloatpxsV64vVRfDFdrjc4VCwKv4611uZOe1N0DOgXhOKRjZf-axR4Btl2XWWJngSKJtgCHJwnSWMZ0PIskxPI06HPddN2vTNoVQWF6gRVAkQPXUEMlJDUxwVDLy3W8a2mesnyWclmwQTl-A-WIv3WH8Ej-KIAyyW4Em2Z5XV5fx1m48JYPzWcj0Qw0yLAABxNwAEEAFcsAVVVmlaLVpPgxMSPk2UlNUjSFWoyZaMfBZn0Y18WSDfsfUEvlfEHViA2SbZ-HoDZwPSe4omCwUp0IwtDINFMTOUrB1M0zDs1w3NwoTCUotLYZYviiy8Rom0bMbezvEc8T6BcvkII8-1Qj8dZfP2R4fXEziUliKTfiIyKK2QIhdCXSEeBYWBXHEbcdL3eQlV6gb8OG0a2FgYkGzsx0HIQfwfJiKJXmSZJoO88IRwAqwP22aD3OeLshP2DrUQi9Ker6jhZqGkaCRESEsJw7A8II6aiFe+aPuW+iHTYCkNqiKxtgHVi+XWYK2SZWqNr5HZslAiNoJSSdvn0rr0rhFh+H4frV3XEQAGEACUAFEVM4OmAH1cAAeRpgBpKglxUqm6dB2yGLWkq0cecrdv2-xDuO1GXluRH9s5H1wKObi7oLNL9WJ0nyYvEQqDpgAZOmqc4Zm2dwAA5OmacFoqRdWcd0asSWkZuoJUbSftpc2vZ6rZDsXg1mTiLzMwOFDsBtPVGR9zgwn9Q6XQo8sglrLtYWIaYzbxZ2lIpZl5IA3O+hLvWeleSiVj6pDgzHpRFODMS7Cc3w8P7q1ypk8jgy0-ve3Vuz9bc+2yWDvO2WrjE2HMcybZYiOHJYm2fIpzYfAIDgTxUrnOhM-ByGAFoEgDE-6CsS+3n8d0nl9aW8enAmHvnEtTyEA+X1F4cOWryM0k5JyTiAZWS3DSKBdIC9zjZDronY8xkyzpjNJ-YqqxXjORXuEfaEYAh9gAtLO4aQNgenqkdSMsCX7wOisuCmlFrwoMdhETazl9hCQ2NLKwFd1gnRiCkMM51og8k2BQruclqFZTMppBhw9RZBldvQTa0FzgdnuF5Y4ZdgrumyI8JyC8RF700E9fqg1gZjWkZDUBWwDgjjElgnawE1GxAUUJPYglwIjjeDGMKCdKGaB1mTF6tD4ArSzpDfabwdiXxApseqtjT6o1SE4txrVXTpHSF4-GnVfF6QjlAKO5ic5RCOn5LsbwjpHWeJyAMZVThHUgvcCSvp9GyRyTgCABT1rhN8rsV2MTYmgQDC6BR7xuznByCcZpYcvqEA6aLSxdxFa2OyCBWIAYimwxXvwoMrp3GrzXkAA */
  createMachine(
    {
      id: "createWorkspaceState",
      predictableActionArguments: true,
      tsTypes: {} as import("./createWorkspaceXService.typegen").Typegen0,
      schema: {
        context: {} as CreateWorkspaceContext,
        events: {} as
          | CreateWorkspaceEvent
          | SelectOwnerEvent
          | RefreshGitAuthEvent,
        services: {} as {
          getTemplates: {
            data: Template[]
          }
          getTemplateGitAuth: {
            data: TemplateVersionGitAuth[]
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
              target: "gettingTemplateGitAuth",
            },
            onError: {
              actions: ["assignCheckPermissionsError"],
            },
          },
        },
        gettingTemplateGitAuth: {
          entry: "clearTemplateGitAuthError",
          invoke: {
            src: "getTemplateGitAuth",
            onDone: {
              actions: ["assignTemplateGitAuth"],
              target: "fillingParams",
            },
            onError: {
              actions: ["assignTemplateGitAuthError"],
              target: "error",
            },
          },
        },
        fillingParams: {
          invoke: {
            id: "listenForRefreshGitAuth",
            src: () => (callback) => {
              // eslint-disable-next-line compat/compat -- It actually is supported... not sure why eslint is complaining.
              const bc = new BroadcastChannel(REFRESH_GITAUTH_BROADCAST_CHANNEL)
              bc.addEventListener("message", () => {
                callback("REFRESH_GITAUTH")
              })
              return () => bc.close()
            },
          },
          on: {
            CREATE_WORKSPACE: {
              actions: ["assignCreateWorkspaceRequest", "assignOwner"],
              target: "creatingWorkspace",
            },
            SELECT_OWNER: {
              actions: ["assignOwner"],
              target: ["fillingParams"],
            },
            REFRESH_GITAUTH: {
              target: "gettingTemplateGitAuth",
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
        getTemplateGitAuth: (context) => {
          const { selectedTemplate } = context

          if (!selectedTemplate) {
            throw new Error("No selected template")
          }

          return getTemplateVersionGitAuth(selectedTemplate.active_version_id)
        },
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
        clearTemplateGitAuthError: assign({
          getTemplateGitAuthError: (_) => undefined,
        }),
        assignTemplateGitAuthError: assign({
          getTemplateGitAuthError: (_, event) => event.data,
        }),
        assignTemplateGitAuth: assign({
          templateGitAuth: (_, event) => event.data,
        }),
      },
    },
  )
