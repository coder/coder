import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"
import { displayError } from "../../components/GlobalSnackbar/utils"

export const Language = {
  getRolesError: "Error on get the roles.",
}

type SiteRolesContext = {
  roles?: TypesGen.AssignableRoles[]
  getRolesError: Error | unknown
}

type SiteRolesEvent = {
  type: "GET_ROLES"
}

export const siteRolesMachine = createMachine(
  {
    id: "siteRolesState",
    predictableActionArguments: true,
    tsTypes: {} as import("./siteRolesXService.typegen").Typegen0,
    schema: {
      context: {} as SiteRolesContext,
      events: {} as SiteRolesEvent,
      services: {
        getRoles: {
          data: {} as TypesGen.AssignableRoles[],
        },
      },
    },
    initial: "idle",
    states: {
      idle: {
        on: {
          GET_ROLES: "gettingRoles",
        },
      },
      gettingRoles: {
        entry: "clearGetRolesError",
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
        displayError(Language.getRolesError)
      },
      clearGetRolesError: assign({
        getRolesError: (_) => undefined,
      }),
    },
    services: {
      getRoles: () => API.getSiteRoles(),
    },
  },
)
