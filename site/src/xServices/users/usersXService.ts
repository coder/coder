import { assign, createMachine } from "xstate";
import * as API from "api/api";
import { getErrorMessage } from "api/errors";
import * as TypesGen from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";

export const Language = {
  updateUserRolesSuccess: "Successfully updated the user roles.",
  updateUserRolesError: "Error on updating the user roles.",
};

export interface UsersContext {
  // Update user roles
  userIdToUpdateRoles?: TypesGen.User["id"];
  updateUserRolesError?: unknown;
}

export type UsersEvent =
  // Update roles events
  {
    type: "UPDATE_USER_ROLES";
    userId: TypesGen.User["id"];
    roles: TypesGen.Role["name"][];
  };

export const usersMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QFdZgE6wMoBcCGOYAdLPujgJYB2UACnlNQRQPZUDEA2gAwC6ioAA4tYFSmwEgAHogC0ARgBMigBxEAnCoCsAdhWrdK9QBYdAGhABPRPO7ruRAMzdj3HYuNbFANm5b1AL4BFqgY2PiERDA4lDQAqmiY7BBsxNQAbiwA1sTRCWE8-EggwqLiVJIyCArc8upOxt6K-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-j-ITHPcS7DXhWc1XMTT-qATGa4jxDoK5kxWjeL0mqIJ5NczwUEIAA */
  createMachine(
    {
      tsTypes: {} as import("./usersXService.typegen").Typegen0,
      schema: {
        context: {} as UsersContext,
        events: {} as UsersEvent,
        services: {} as {
          updateUserRoles: {
            data: TypesGen.User;
          };
        },
      },
      predictableActionArguments: true,
      id: "usersState",
      initial: "idle",
      states: {
        idle: {
          entry: "clearSelectedUser",
          on: {
            UPDATE_USER_ROLES: {
              target: "updatingUserRoles",
              actions: "assignUserIdToUpdateRoles",
            },
          },
        },
        updatingUserRoles: {
          entry: "clearUpdateUserRolesError",
          invoke: {
            src: "updateUserRoles",
            id: "updateUserRoles",
            onDone: [
              {
                target: "idle",
                actions: "updateUserRolesInTheList",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: [
                  "assignUpdateRolesError",
                  "displayUpdateRolesErrorMessage",
                ],
              },
            ],
          },
        },
      },
    },
    {
      services: {
        updateUserRoles: (context, event) => {
          if (!context.userIdToUpdateRoles) {
            throw new Error("userIdToUpdateRoles is undefined");
          }

          return API.updateUserRoles(event.roles, context.userIdToUpdateRoles);
        },
      },

      actions: {
        clearSelectedUser: assign({
          userIdToUpdateRoles: (_) => undefined,
        }),

        assignUserIdToUpdateRoles: assign({
          userIdToUpdateRoles: (_, event) => event.userId,
        }),

        assignUpdateRolesError: assign({
          updateUserRolesError: (_, event) => event.data,
        }),

        clearUpdateUserRolesError: assign({
          updateUserRolesError: (_) => undefined,
        }),
        displayUpdateRolesErrorMessage: (context) => {
          const message = getErrorMessage(
            context.updateUserRolesError,
            Language.updateUserRolesError,
          );
          displayError(message);
        },
        updateUserRolesInTheList: assign({
          // users: ({ users }, event) => {
          //   if (!users) {
          //     return users;
          //   }
          //   return users.map((u) => {
          //     return u.id === event.data.id ? event.data : u;
          //   });
          // },
        }),
      },
    },
  );
