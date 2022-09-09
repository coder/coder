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

export const templatesMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QBcwFsAOAbAhq2AysnmAHQzLICWAdlAPIBOUONVAXnlQPY2wDEEXmVoA3bgGsyFJizadqveEhAZusKopqJQAD0QAWAMwHSATgDsANgCsNgzeuOATABoQAT0QBGb89I2AAzB3oEAHH7eFtFhAL6x7qiYuPhEJORglLQMzKwcXEr8YIyM3Iyk2HgAZmVoGciyeQo8fDqq6potbfoIxqaWtvaOthZunogmRgHBgc4Wvs5mYbPxieiVqcSo9dR0ACrrKXCCwqRiktKZB8kkyqBqGlrdPsH9Zs6+VlZhxlbOBu4vAgzIFSDNgot5s4wnZViAkhs4GlthRdlBroiBMVSuUNjVGHUKBijnd2o8uioeqFAm8Pt4vj8jH8AeMEM5AjYwTMouEDGEzPTnPEEiAaNwIHA2giScjLlk6I15AVWioHp1eM9emMgb4zNMZh8wvMzH8LHDpbdZTtssTbm01U9KYgvqDmUsIt4+TYIoDECbzGZA2Y+lYjIEjIHzYdLVsyEIaGB7R1HXofOyLKQDIsrKELAYogKjL6EL4wlyZjCswYHBEozdNulsWUk+SNU6S+nM9nc-mLIXi845qQjCOjDZg3YwjC65jZS31dp294wsXl8LYkA */
  createMachine(
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
          getPermissions: {
            data: boolean
          }
          getTemplates: {
            data: TypesGen.Template[]
          }
        },
      },
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
            onDone: [
              {
                actions: ["assignTemplates", "clearTemplatesError"],
                target: "done",
              },
            ],
            onError: [
              {
                actions: "assignTemplatesError",
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
