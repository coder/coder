import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as TypesGen from "../../api/typesGenerated"
import { displayError } from "../../components/GlobalSnackbar/utils"

type SiteRolesContext = {
  roles?: TypesGen.Role[]
  getRolesError: Error | unknown
}

type SiteRolesEvent = {
  type: "GET_ROLES"
  organizationId: string
}

export const siteRolesMachine = createMachine(
  {
    id: "siteRolesState",
    initial: "idle",
    schema: {
      context: {} as SiteRolesContext,
      events: {} as SiteRolesEvent,
      services: {
        getRoles: {
          data: {} as TypesGen.Role[],
        },
      },
    },
    tsTypes: {} as import("./siteRolesXService.typegen").Typegen0,
    states: {
      idle: {
        on: {
          GET_ROLES: "gettingRoles",
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
      displayGetRolesError: () => {
        displayError("Error on get the roles.")
      },
    },
    services: {
      getRoles: () => API.getSiteRoles(),
    },
  },
)
