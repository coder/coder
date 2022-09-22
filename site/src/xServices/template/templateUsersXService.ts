import { getTemplateUserRoles, updateTemplateMeta } from "api/api"
import { TemplateRole, TemplateUser, User } from "api/typesGenerated"
import { displaySuccess } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"

export const templateUsersMachine = createMachine(
  {
    schema: {
      context: {} as {
        templateId: string
        templateUsers?: TemplateUser[]
        userToBeAdded?: TemplateUser
        userToBeUpdated?: TemplateUser
        addUserCallback?: () => void
      },
      services: {} as {
        loadTemplateUsers: {
          data: TemplateUser[]
        }
        addUser: {
          data: unknown
        }
        updateUser: {
          data: unknown
        }
      },
      events: {} as
        | {
            type: "ADD_USER"
            user: User
            role: TemplateRole
            onDone: () => void
          }
        | {
            type: "UPDATE_USER_ROLE"
            user: User
            role: TemplateRole
          }
        | {
            type: "REMOVE_USER"
            user: User
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
          UPDATE_USER_ROLE: { target: "updatingUser", actions: ["assignUserToBeUpdated"] },
          REMOVE_USER: { target: "removingUser", actions: ["removeUserFromTemplateUsers"] },
        },
      },
      addingUser: {
        invoke: {
          src: "addUser",
          onDone: {
            target: "idle",
            actions: ["addUserToTemplateUsers", "runAddCallback"],
          },
        },
      },
      updatingUser: {
        invoke: {
          src: "updateUser",
          onDone: {
            target: "idle",
            actions: [
              "updateUserOnTemplateUsers",
              "clearUserToBeUpdated",
              "displayUpdateSuccessMessage",
            ],
          },
        },
      },
      removingUser: {
        invoke: {
          src: "removeUser",
          onDone: {
            target: "idle",
            actions: ["displayRemoveSuccessMessage"],
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
      updateUser: ({ templateId }, { user, role }) =>
        updateTemplateMeta(templateId, {
          user_perms: {
            [user.id]: role,
          },
        }),
      removeUser: ({ templateId }, { user }) =>
        updateTemplateMeta(templateId, {
          user_perms: {
            [user.id]: "",
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
      runAddCallback: ({ addUserCallback }) => {
        if (addUserCallback) {
          addUserCallback()
        }
      },
      assignUserToBeUpdated: assign({
        userToBeUpdated: (_, { user, role }) => ({ ...user, role }),
      }),
      updateUserOnTemplateUsers: assign({
        templateUsers: ({ templateUsers, userToBeUpdated }) => {
          if (!templateUsers || !userToBeUpdated) {
            throw new Error("No user to be updated.")
          }
          return templateUsers.map((oldTemplateUser) => {
            return oldTemplateUser.id === userToBeUpdated.id ? userToBeUpdated : oldTemplateUser
          })
        },
      }),
      clearUserToBeUpdated: assign({
        userToBeUpdated: (_) => undefined,
      }),
      displayUpdateSuccessMessage: () => {
        displaySuccess("Collaborator role update successfully!")
      },
      removeUserFromTemplateUsers: assign({
        templateUsers: ({ templateUsers }, { user }) => {
          if (!templateUsers) {
            throw new Error("No user to be removed.")
          }
          return templateUsers.filter((oldTemplateUser) => {
            return oldTemplateUser.id !== user.id
          })
        },
      }),
      displayRemoveSuccessMessage: () => {
        displaySuccess("Collaborator removed successfully!")
      },
    },
  },
)
