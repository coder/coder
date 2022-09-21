import { getTemplateUserRoles, updateTemplateMeta } from "api/api"
import { TemplateRole, TemplateUser, User } from "api/typesGenerated"
import { assign, createMachine } from "xstate"

export const templateUsersMachine = createMachine(
  {
    schema: {
      context: {} as {
        templateId: string
        templateUsers?: TemplateUser[]
        userToBeAdded?: TemplateUser
        addUserCallback?: () => void
      },
      services: {} as {
        loadTemplateUsers: {
          data: TemplateUser[]
        }
        addUser: {
          data: unknown
        }
      },
      events: {} as {
        type: "ADD_USER"
        user: User
        role: TemplateRole
        onDone: () => void
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
            target: "idle",
          },
        },
      },
      idle: {
        on: {
          ADD_USER: { target: "addingUser", actions: ["assignUserToBeAdded"] },
        },
      },
      addingUser: {
        invoke: {
          src: "addUser",
          onDone: {
            target: "idle",
            actions: ["addUserToTemplateUsers", "runCallback"],
          },
        },
      },
    },
  },
  {
    services: {
      loadTemplateUsers: ({ templateId }) => getTemplateUserRoles(templateId),
      addUser: ({ templateId }, { user, role }) =>
        updateTemplateMeta(templateId, {
          user_perms: {
            [user.id]: role,
          },
        }),
    },
    actions: {
      assignTemplateUsers: assign({
        templateUsers: (_, { data }) => data,
      }),
      assignUserToBeAdded: assign({
        userToBeAdded: (_, { user, role }) => ({ ...user, role }),
        addUserCallback: (_, { onDone }) => onDone,
      }),
      addUserToTemplateUsers: assign({
        templateUsers: ({ templateUsers = [], userToBeAdded }) => {
          if (!userToBeAdded) {
            throw new Error("No user to be added")
          }
          return [...templateUsers, userToBeAdded]
        },
      }),
      runCallback: ({ addUserCallback }) => {
        if (addUserCallback) {
          addUserCallback()
        }
      },
    },
  },
)
