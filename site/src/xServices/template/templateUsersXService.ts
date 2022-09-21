import { getTemplateUserRoles } from "api/api"
import { TemplateUser } from "api/typesGenerated"
import { assign, createMachine } from "xstate"

export const templateUsersMachine = createMachine(
  {
    schema: {
      context: {} as {
        templateId: string
        templateUsers?: TemplateUser[]
      },
      services: {} as {
        loadTemplateUsers: {
          data: TemplateUser[]
        }
      },
    },
    tsTypes: {} as import("./templateUsersXService.typegen").Typegen0,
    id: "templateUserRoles",
    initial: "loading",
    states: {
      loading: {
        invoke: {
          src: "loadTemplateUsers",
          onDone: {
            actions: ["assignTemplateUsers"],
            target: "success",
          },
        },
      },
      success: {
        type: "final",
      },
    },
  },
  {
    services: {
      loadTemplateUsers: ({ templateId }) => getTemplateUserRoles(templateId),
    },
    actions: {
      assignTemplateUsers: assign({
        templateUsers: (_, { data }) => data,
      }),
    },
  },
)
