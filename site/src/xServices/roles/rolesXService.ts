import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as TypesGen from "../../api/typesGenerated"
import { displayError } from "../../components/GlobalSnackbar/utils"

type RolesContext = {
  roles?: TypesGen.Role[]
  getRolesError: Error | unknown
  organizationId?: string
}

type RolesEvent = {
  type: "GET_ROLES"
  organizationId: string
}

export const rolesMachine = createMachine(
  {
    id: "rolesState",
    initial: "idle",
    schema: {
      context: {} as RolesContext,
      events: {} as RolesEvent,
      services: {
        getRoles: {
          data: {} as TypesGen.Role[],
        },
      },
    },
    tsTypes: {} as import("./rolesXService.typegen").Typegen0,
    states: {
      idle: {
        on: {
          GET_ROLES: {
            target: "gettingRoles",
            actions: ["assignOrganizationId"],
          },
        },
      },
      gettingRoles: {
        invoke: {
          id: "getRoles",
          src: "getRoles",
          onDone: {
            target: "idle",
            actions: ["assignRoles"],
          },
          onError: {
            target: "idle",
            actions: ["assignGetRolesError", "displayGetRolesError"],
          },
        },
      },
    },
  },
  {
    actions: {
      assignRoles: assign({
        roles: (_, event) => event.data,
      }),
      assignGetRolesError: assign({
        getRolesError: (_, event) => event.data,
      }),
      assignOrganizationId: assign({
        organizationId: (_, event) => event.organizationId,
      }),
      displayGetRolesError: () => {
        displayError("Error on get the roles.")
      },
    },
    services: {
      getRoles: (ctx) => {
        const { organizationId } = ctx

        if (!organizationId) {
          throw new Error("organizationId not defined")
        }

        return API.getOrganizationRoles(organizationId)
      },
    },
  },
)
