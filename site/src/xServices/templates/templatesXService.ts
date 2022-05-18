import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"

interface TemplatesContext {
  organizations?: TypesGen.Organization[]
  templates?: TypesGen.Template[]
  canCreateTemplate?: boolean
  permissionsError?: Error | unknown
  organizationsError?: Error | unknown
  templatesError?: Error | unknown
}

export const templatesMachine = createMachine(
  {
    tsTypes: {} as import("./templatesXService.typegen").Typegen0,
    schema: {
      context: {} as TemplatesContext,
      services: {} as {
        getOrganizations: {
          data: TypesGen.Organization[]
        }
        getPermissions: {
          data: boolean
        }
        getTemplates: {
          data: TypesGen.Template[]
        }
      },
    },
    id: "templatesState",
    initial: "gettingOrganizations",
    states: {
      gettingOrganizations: {
        entry: "clearOrganizationsError",
        invoke: {
          src: "getOrganizations",
          id: "getOrganizations",
          onDone: [
            {
              actions: ["assignOrganizations", "clearOrganizationsError"],
              target: "gettingTemplates",
            },
          ],
          onError: [
            {
              actions: "assignOrganizationsError",
              target: "error",
            },
          ],
        },
        tags: "loading",
      },
      gettingTemplates: {
        entry: "clearTemplatesError",
        invoke: {
          src: "getTemplates",
          id: "getTemplates",
          onDone: {
            target: "done",
            actions: ["assignTemplates", "clearTemplatesError"],
          },
          onError: {
            target: "error",
            actions: "assignTemplatesError",
          },
        },
        tags: "loading",
      },
      done: {},
      error: {},
    },
  },
  {
    actions: {
      assignOrganizations: assign({
        organizations: (_, event) => event.data,
      }),
      assignOrganizationsError: assign({
        organizationsError: (_, event) => event.data,
      }),
      clearOrganizationsError: assign((context) => ({
        ...context,
        organizationsError: undefined,
      })),
      assignTemplates: assign({
        templates: (_, event) => event.data,
      }),
      assignTemplatesError: assign({
        templatesError: (_, event) => event.data,
      }),
      clearTemplatesError: (context) => assign({ ...context, getWorkspacesError: undefined }),
    },
    services: {
      getOrganizations: API.getOrganizations,
      getTemplates: async (context) => {
        if (!context.organizations || context.organizations.length === 0) {
          throw new Error("no organizations")
        }
        return API.getTemplates(context.organizations[0].id)
      },
    },
  },
)
