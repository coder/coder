import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"

interface TemplatesContext {
  organizations?: TypesGen.Organization[]
  templates?: TypesGen.Template[]
  canCreateTemplate?: boolean
  getOrganizationsError?: Error | unknown
  getTemplatesError?: Error | unknown
}

export const templatesMachine = createMachine(
  {
    id: "templatesState",
    predictableActionArguments: true,
    tsTypes: {} as import("./templatesXService.typegen").Typegen0,
    schema: {
      context: {} as TemplatesContext,
      services: {} as {
        getOrganizations: {
          data: TypesGen.Organization[]
        }
        getTemplates: {
          data: TypesGen.Template[]
        }
      },
    },
    initial: "gettingOrganizations",
    states: {
      gettingOrganizations: {
        entry: "clearGetOrganizationsError",
        invoke: {
          src: "getOrganizations",
          id: "getOrganizations",
          onDone: {
            actions: ["assignOrganizations"],
            target: "gettingTemplates",
          },
          onError: {
            actions: "assignGetOrganizationsError",
            target: "error",
          },
        },
        tags: "loading",
      },
      gettingTemplates: {
        entry: "clearGetTemplatesError",
        invoke: {
          src: "getTemplates",
          id: "getTemplates",
          onDone: {
            actions: "assignTemplates",
            target: "done",
          },
          onError: {
            actions: "assignGetTemplatesError",
            target: "error",
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
      assignGetOrganizationsError: assign({
        getOrganizationsError: (_, event) => event.data,
      }),
      clearGetOrganizationsError: assign((context) => ({
        ...context,
        getOrganizationsError: undefined,
      })),
      assignTemplates: assign({
        templates: (_, event) => event.data,
      }),
      assignGetTemplatesError: assign({
        getTemplatesError: (_, event) => event.data,
      }),
      clearGetTemplatesError: (context) =>
        assign({ ...context, getTemplatesError: undefined }),
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
