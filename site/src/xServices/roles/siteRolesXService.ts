import { assign, createMachine } from "xstate";
import * as API from "../../api/api";
import * as TypesGen from "../../api/typesGenerated";
import { displayError } from "../../components/GlobalSnackbar/utils";

export const Language = {
  getRolesError: "Error on get the roles.",
};

type SiteRolesContext = {
  hasPermission: boolean;
  roles?: TypesGen.AssignableRoles[];
  getRolesError: unknown;
};

export const siteRolesMachine = createMachine(
  {
    id: "siteRolesState",
    predictableActionArguments: true,
    tsTypes: {} as import("./siteRolesXService.typegen").Typegen0,
    schema: {
      context: {} as SiteRolesContext,
      services: {
        getRoles: {
          data: {} as TypesGen.AssignableRoles[],
        },
      },
    },
    initial: "initializing",
    states: {
      initializing: {
        always: [
          { target: "gettingRoles", cond: "hasPermission" },
          { target: "done" },
        ],
      },
      gettingRoles: {
        entry: "clearGetRolesError",
        invoke: {
          id: "getRoles",
          src: "getRoles",
          onDone: {
            target: "done",
            actions: ["assignRoles"],
          },
          onError: {
            target: "done",
            actions: ["assignGetRolesError", "displayGetRolesError"],
          },
        },
      },
      done: {
        type: "final",
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
        displayError(Language.getRolesError);
      },
      clearGetRolesError: assign({
        getRolesError: (_) => undefined,
      }),
    },
    services: {
      getRoles: () => API.getSiteRoles(),
    },
    guards: {
      hasPermission: ({ hasPermission }) => hasPermission,
    },
  },
);
