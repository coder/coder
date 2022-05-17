import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"

interface TemplateContext {
  name: string

  organizations?: TypesGen.Organization[]
  organizationsError?: Error | unknown
  template?: TypesGen.Template
  templateError?: Error | unknown
}

export const templateMachine = createMachine(
  {
    tsTypes: {} as import("./templateXService.typegen").Typegen0,
    schema: {
      context: {} as TemplateContext,
      services: {} as {
        getOrganizations: {
          data: TypesGen.Organization[]
        }
        getPermissions: {
          data: boolean
        }
        getTemplate: {
          data: TypesGen.Template
        }
      },
    },
    id: "templateState",
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
              target: "gettingTemplate",
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
      gettingTemplate: {
        entry: "clearTemplateError",
        invoke: {
          src: "getTemplate",
          id: "getTemplate",
          onDone: {
            target: "done",
            actions: ["assignTemplate", "clearTemplateError"],
          },
          onError: {
            target: "error",
            actions: "assignTemplateError",
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
      assignTemplate: assign({
        template: (_, event) => event.data,
      }),
      assignTemplateError: assign({
        templateError: (_, event) => event.data,
      }),
      clearTemplateError: (context) => assign({ ...context, getWorkspacesError: undefined }),
    },
    services: {
      getOrganizations: API.getOrganizations,
      getTemplate: async (context) => {
        if (!context.organizations || context.organizations.length === 0) {
          throw new Error("no organizations")
        }
        return API.getTemplateByName(context.organizations[0].id, context.name)
      },
    },
  },
)
