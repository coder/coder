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

export const templatesMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QBcwFsAOAbAhq2AysnmAHQzLICWAdlAPIBOUONVAXnlQPY2wDEEXmVoA3bgGsyFJizadqveEhAZusKopqJQAD0QAWAMwHSATgDsANgCsAJgt2rRuwAYAHBYsAaEAE9EAEZAu1IDMwiIm3dXG0DjdwBfRN9UTFx8IhJyMEpaBmZWDi4lfjBGRm5GUmw8ADMqtBzkWSKFHj4dVXVNDq79BGNTS1sHJxcPL18AhBMjUhtXJaWrOzM7O3cDG2TU9FrM4lRm6joAFX2MuEFhUjFJaVyL9JJlUDUNLX6gpeH1wJcIQs7giVmmiBB5kilgM7iMHmcOxSIDSBzgWWOFFOUGeaIE5Uq1QODUYTQouKub26nz6KgGgV+ULsAOZDhBZjB-kQbhspGWSxC0W2VncSORNG4EDgXVRlIxjzydFa8hKnRUH16vG+gzs4IQwTMYWhjgsRjm9l2KMur3lJ3yFNeXQ1XzpiCsVlcpFW4QMFnisJZeo5UMicQsNjMgRsSL2L0O2SENDATp6Lr0QTcFjCa2cfqsgRNeuC7j5-Ncq3WmwMgUtsptRzIBKqKZpWtd+sz2Y5RjzBYceo2WbNw9hrijZtr1vjqBbmu07cC7iLSWSiSAA */
  createMachine(
    {
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
      id: "templatesState",
      initial: "gettingOrganizations",
      states: {
        gettingOrganizations: {
          entry: "clearGetOrganizationsError",
          invoke: {
            src: "getOrganizations",
            id: "getOrganizations",
            onDone: [
              {
                actions: ["assignOrganizations"],
                target: "gettingTemplates",
              },
            ],
            onError: [
              {
                actions: "assignGetOrganizationsError",
                target: "error",
              },
            ],
          },
          tags: "loading",
        },
        gettingTemplates: {
          entry: "clearGetTemplatesError",
          invoke: {
            src: "getTemplates",
            id: "getTemplates",
            onDone: [
              {
                actions: ["assignTemplates"],
                target: "done",
              },
            ],
            onError: [
              {
                actions: "assignGetTemplatesError",
                target: "error",
              },
            ],
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
        clearGetTemplatesError: (context) => assign({ ...context, getTemplatesError: undefined }),
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
