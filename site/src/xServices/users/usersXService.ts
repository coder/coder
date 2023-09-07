import { getPaginationData } from "components/PaginationWidget/utils";
import {
  PaginationContext,
  paginationMachine,
  PaginationMachineRef,
} from "xServices/pagination/paginationXService";
import { assign, createMachine, send, spawn } from "xstate";
import * as API from "../../api/api";
import { getErrorMessage } from "../../api/errors";
import * as TypesGen from "../../api/typesGenerated";
import {
  displayError,
  displaySuccess,
} from "../../components/GlobalSnackbar/utils";
import { queryToFilter } from "../../utils/filters";
import { generateRandomString } from "../../utils/random";

const usersPaginationId = "usersPagination";

export const Language = {
  getUsersError: "Error getting users.",
  suspendUserSuccess: "Successfully suspended the user.",
  suspendUserError: "Error suspending user.",
  deleteUserSuccess: "Successfully deleted the user.",
  deleteUserError: "Error deleting user.",
  activateUserSuccess: "Successfully activated the user.",
  activateUserError: "Error activating user.",
  resetUserPasswordSuccess: "Successfully updated the user password.",
  resetUserPasswordError: "Error on resetting the user password.",
  updateUserRolesSuccess: "Successfully updated the user roles.",
  updateUserRolesError: "Error on updating the user roles.",
};

export interface UsersContext {
  // Get users
  users?: TypesGen.User[];
  filter: string;
  getUsersError?: unknown;
  // Suspend user
  userIdToSuspend?: TypesGen.User["id"];
  usernameToSuspend?: TypesGen.User["username"];
  suspendUserError?: unknown;
  // Delete user
  userIdToDelete?: TypesGen.User["id"];
  usernameToDelete?: TypesGen.User["username"];
  deleteUserError?: unknown;
  // Activate user
  userIdToActivate?: TypesGen.User["id"];
  usernameToActivate?: TypesGen.User["username"];
  activateUserError?: unknown;
  // Reset user password
  userIdToResetPassword?: TypesGen.User["id"];
  resetUserPasswordError?: unknown;
  newUserPassword?: string;
  // Update user roles
  userIdToUpdateRoles?: TypesGen.User["id"];
  updateUserRolesError?: unknown;
  paginationContext: PaginationContext;
  paginationRef: PaginationMachineRef;
  count: number;
}

export type UsersEvent =
  | { type: "GET_USERS"; query?: string }
  // Suspend events
  | {
      type: "SUSPEND_USER";
      userId: TypesGen.User["id"];
      username: TypesGen.User["username"];
    }
  | { type: "CONFIRM_USER_SUSPENSION" }
  | { type: "CANCEL_USER_SUSPENSION" }
  // Delete events
  | {
      type: "DELETE_USER";
      userId: TypesGen.User["id"];
      username: TypesGen.User["username"];
    }
  | { type: "CONFIRM_USER_DELETE" }
  | { type: "CANCEL_USER_DELETE" }
  // Activate events
  | {
      type: "ACTIVATE_USER";
      userId: TypesGen.User["id"];
      username: TypesGen.User["username"];
    }
  | { type: "CONFIRM_USER_ACTIVATION" }
  | { type: "CANCEL_USER_ACTIVATION" }
  // Reset password events
  | { type: "RESET_USER_PASSWORD"; userId: TypesGen.User["id"] }
  | { type: "CONFIRM_USER_PASSWORD_RESET" }
  | { type: "CANCEL_USER_PASSWORD_RESET" }
  // Update roles events
  | {
      type: "UPDATE_USER_ROLES";
      userId: TypesGen.User["id"];
      roles: TypesGen.Role["name"][];
    }
  // Filter
  | { type: "UPDATE_FILTER"; query: string }
  // Pagination
  | { type: "UPDATE_PAGE"; page: string };

export const usersMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QFdZgE6wMoBcCGOYAdLPujgJYB2UACnlNQRQPZUDEA2gAwC6ioAA4tYFSmwEgAHogC0ARgBMigBxEAnCoCsAdhWrdK9QBYdAGhABPRPO7ruRAMzdj3HYuNbFANm5b1AL4BFqgY2PiERDA4lDQAqmiY7BBsxNQAbiwA1sTRCWE8-EggwqLiVJIyCArc8upOxt6K-o7qjloq3ioW1ghKPkQu6m2KOraaw0EhieEEuWAx1FD5SRjoLOhEggA2BABmGwC2UQsrsIWSpWKsFcVVCqqOGoqOKnUqKrW15lY2ivI6IiNRqOHR6bhdXRTEChTC4OZECgQbZgdhYOJYWgAUQAcgARAD6GKxACULsUruVKnIdLoiPJjPItF42p0fI4ejYtMZjE4ASoxi4dI4lCpobDZpEkSj2HisQAZLEAFSxRKwpPJQhE1wkdxp3nqAJM3i0zm8tPknL63gNRB0TQBTNBHRM4pm8KlyNRAEEAMJKgCSADVvSq1Rq+JdtVS9dUdMYnkN1C9uM0Teofr15CpXEQPt95IXC952m6wh60l72CSseqleGSQTaN6sFgAOoAeRJeM1JWjN2pca8RH+bmar3tdUUVve3me8kcIqU3G0NrLcIilZlcVoeNDquJjZJHcVWF7lIHsdk8d5As6ybeHxLxitym4Dh5b1pYx0LhUjnXSUt1RHc9zDZsAHEsXPftdVAe4GT0IEMz0UYtAhV51BnZwHDsTQDQXZNNEAitESrUD9wJAAxAN5RVMlIwpWDbnguR5BNXlEPUbNmk+Ixp1+PpFHULQgUcbxTH8YTCy8EjNyIABjNg9godBDhWLBUEEMAqFENh2F9DscRokkAFkGwJdFMVxLAAyMmCykvVjqg8eQ82UdQJJ5DxaWZGdmUUDRWi8VM+LGbw5IRJSqBUtSNK0nS9I4X1vRxX0FQsqzsRxWz7MYrVHLg6Q5GMbQgWZbiS3tbhWktQT2P+PMBR0DMM0dLQIuCGF3Xk6LYvUxI8TAFFygMoyTPMw8CTlRUVQcnUWOKlzlEGBlHATPRhn0JkZy6XlSuMUZamEox7UiyI+tUgaMCGkabgM1L0vlCyZuVaD8r7QrFvuYwM3pewRUhTxPkzGx7XqYTGTayTtEUc7iEuuLEm9BTKHSZh9MM4yAzMiy-UDENAzyooCoWwdZBeNQfBccT0NqbkOhnHNAXaPxVHsJpTB0eHFOUq6VhRtGMeSx6Mqm-Hg1DOycXmmNnNkdC51KlqlHaYTRgErM2lvZkDUO37-ELHnYASqgICWFZklSREqEyHISFNiAVllpyltaJ5fscXivd0UqsPq0q3MLFrzRzV5MONx2LcSdg1g2LZdhwA41Id2BtLN52PovIqqhNUTOgI2xF3tUEZy5kcXXNQ6Vx8TrpnLeSIGGhZo4wK2qDSW3smIJuRrATOSc+snY1cD83hC1lPG8OqswkiHGghX8Du0Ywed7lv4hjuPNh2fYjiIdfCAHqMvsHc03PfdpPI6xkF26eqS1E5pUNUbQM1GHm8FRih0diZYY5SB3G2dtiBfyFkfRILsc6IGCiOX8vgmTsTGJ5F89UwQOBDtoU0pgPCry6hKUiYCf7ME3m3beCc94pyIb-fukCs7MTPtPIgCDhJuE+O7bwM49BPBFCYXQoxhjsTFPgnqUU+ZIwwPQWAsAADuGwIAkjgAsMa2NcZTWbK2Ts3YCQ1jrFA76bEPiDFaP+G0UkXhaFfLUNQWhsx6xeM4dweD64bjETFfmiQpGyPkYotAOAHppTFuqRsGj2xdkJLo5U+jya2MBDTWkyhaQGk6K+WwolGjuH+KVaeDIeboCUYsUh6AvFyPQBAduncQFEHyX4lYJT5HRNjKCD2RYRIgyaIoPwM5uJuXEsKDqjwVZ5IKX-OpeBpGlPKeQ3eSd941NOJ48Z3iymNOchJJ4bRFyXy6MmLo3TPIaAFJ0xcug3CMh5sgQQEASH-wwCSFgKJYAVOAd3IglzrkQLuQ8uAqy3adAaHYdiSgOivBnogf4tpWhbQBK4ZQuSRENwRO8m5Kx7mPNjugdYO9E7J2OMiz56A0U-PoafWMJo3I+AZLXcc7FLGCThXObiYxRie2DvDdgFEww0TohGQe2cDHVFsAmekvgTnsV+mkq09gHDOHWs0TpU5OpdSoCwJu8BigEPkqQPA5Alj0EYFQYWJ9h7ywXAyPMdgEzV02bSKVC47SfHfLSX65pSwItcZEaIoyZjGrlktBQSh6guAhI0aejJhQzjcKJA0Xt2g8i8Aqnm0owC+tdghO+eYTRoSLo0DM2F4xEGwZ00wbg1bc3dUBXm7iJHoE0mnRKrt+XkwlcY0wrI6jZlqP5YcbRjQeDsLoM6FbSKI2uugW6LcipNqvNk5hph1rTw6HUPZD8cxAhzEdQNp1nHdURRdcRY7BbEL9dO+WpVo2lSjQKEUjJ2hM25KtNoisvaHXNJHetZtW7oFTdAhAbxmF2HjKCIwIl1b+SZGJES75RimIBGvZu3qMA-oFQKO0edxJVW8kYXazI7ToT9nTAUrph3yWoSixIyHBxlQkrYaDnwOqaFBn0N4c5bGsm0MdcYPNR1jImT4gplGZ0SXpKaAdYJQRtHNFYjwThWoLg8JVESwy-GIeKUsyZgnnIMjcuGuVEIl2mCsamJwXsBR2BeOKuuu6PXEHxV+ol6rSZ+qqCKQYyYMNoRBsKVBvR-j-ITHPcS7DXhWc1XMTT-qATGa4jxDoK5kxWjeL0mqIJ5NczwUEIAA */
  createMachine(
    {
      tsTypes: {} as import("./usersXService.typegen").Typegen0,
      schema: {
        context: {} as UsersContext,
        events: {} as UsersEvent,
        services: {} as {
          getUsers: {
            data: TypesGen.GetUsersResponse;
          };
          createUser: {
            data: TypesGen.User;
          };
          suspendUser: {
            data: TypesGen.User;
          };
          deleteUser: {
            data: undefined;
          };
          activateUser: {
            data: TypesGen.User;
          };
          updateUserPassword: {
            data: undefined;
          };
          updateUserRoles: {
            data: TypesGen.User;
          };
        },
      },
      predictableActionArguments: true,
      id: "usersState",
      on: {
        UPDATE_FILTER: {
          actions: ["assignFilter", "sendResetPage"],
        },
        UPDATE_PAGE: {
          target: "gettingUsers",
          actions: "updateURL",
        },
      },
      initial: "startingPagination",
      states: {
        startingPagination: {
          entry: "assignPaginationRef",
          always: {
            target: "gettingUsers",
          },
        },
        gettingUsers: {
          entry: "clearGetUsersError",
          invoke: {
            src: "getUsers",
            id: "getUsers",
            onDone: [
              {
                target: "idle",
                actions: "assignUsers",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: ["clearUsers", "assignGetUsersError"],
              },
            ],
          },
          tags: "loading",
        },
        idle: {
          entry: "clearSelectedUser",
          on: {
            SUSPEND_USER: {
              target: "confirmUserSuspension",
              actions: "assignUserToSuspend",
            },
            DELETE_USER: {
              target: "confirmUserDeletion",
              actions: "assignUserToDelete",
            },
            ACTIVATE_USER: {
              target: "confirmUserActivation",
              actions: "assignUserToActivate",
            },
            RESET_USER_PASSWORD: {
              target: "confirmUserPasswordReset",
              actions: [
                "assignUserIdToResetPassword",
                "generateRandomPassword",
              ],
            },
            UPDATE_USER_ROLES: {
              target: "updatingUserRoles",
              actions: "assignUserIdToUpdateRoles",
            },
          },
        },
        confirmUserSuspension: {
          on: {
            CONFIRM_USER_SUSPENSION: {
              target: "suspendingUser",
            },
            CANCEL_USER_SUSPENSION: {
              target: "idle",
            },
          },
        },
        confirmUserDeletion: {
          on: {
            CONFIRM_USER_DELETE: {
              target: "deletingUser",
            },
            CANCEL_USER_DELETE: {
              target: "idle",
            },
          },
        },
        confirmUserActivation: {
          on: {
            CONFIRM_USER_ACTIVATION: {
              target: "activatingUser",
            },
            CANCEL_USER_ACTIVATION: {
              target: "idle",
            },
          },
        },
        suspendingUser: {
          entry: "clearSuspendUserError",
          invoke: {
            src: "suspendUser",
            id: "suspendUser",
            onDone: [
              {
                target: "gettingUsers",
                actions: "displaySuspendSuccess",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: [
                  "assignSuspendUserError",
                  "displaySuspendedErrorMessage",
                ],
              },
            ],
          },
        },
        deletingUser: {
          entry: "clearDeleteUserError",
          invoke: {
            src: "deleteUser",
            id: "deleteUser",
            onDone: [
              {
                target: "gettingUsers",
                actions: "displayDeleteSuccess",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: ["assignDeleteUserError", "displayDeleteErrorMessage"],
              },
            ],
          },
        },
        activatingUser: {
          entry: "clearActivateUserError",
          invoke: {
            src: "activateUser",
            id: "activateUser",
            onDone: [
              {
                target: "gettingUsers",
                actions: "displayActivateSuccess",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: [
                  "assignActivateUserError",
                  "displayActivatedErrorMessage",
                ],
              },
            ],
          },
        },
        confirmUserPasswordReset: {
          on: {
            CONFIRM_USER_PASSWORD_RESET: {
              target: "resettingUserPassword",
            },
            CANCEL_USER_PASSWORD_RESET: {
              target: "idle",
            },
          },
        },
        resettingUserPassword: {
          entry: "clearResetUserPasswordError",
          invoke: {
            src: "resetUserPassword",
            id: "resetUserPassword",
            onDone: [
              {
                target: "idle",
                actions: "displayResetPasswordSuccess",
              },
            ],
            onError: [
              {
                target: "idle",
                actions: [
                  "assignResetUserPasswordError",
                  "displayResetPasswordErrorMessage",
                ],
              },
            ],
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
        // Passing API.getUsers directly does not invoke the function properly
        // when it is mocked. This happen in the UsersPage tests inside of the
        // "shows a success message and refresh the page" test case.
        getUsers: (context) => {
          const { offset, limit } = getPaginationData(context.paginationRef);
          return API.getUsers({
            ...queryToFilter(context.filter),
            offset,
            limit,
          });
        },
        suspendUser: (context) => {
          if (!context.userIdToSuspend) {
            throw new Error("userIdToSuspend is undefined");
          }

          return API.suspendUser(context.userIdToSuspend);
        },
        deleteUser: (context) => {
          if (!context.userIdToDelete) {
            throw new Error("userIdToDelete is undefined");
          }
          return API.deleteUser(context.userIdToDelete);
        },
        activateUser: (context) => {
          if (!context.userIdToActivate) {
            throw new Error("userIdToActivate is undefined");
          }

          return API.activateUser(context.userIdToActivate);
        },
        resetUserPassword: (context) => {
          if (!context.userIdToResetPassword) {
            throw new Error("userIdToResetPassword is undefined");
          }

          if (!context.newUserPassword) {
            throw new Error("newUserPassword not generated");
          }

          return API.updateUserPassword(context.userIdToResetPassword, {
            password: context.newUserPassword,
            old_password: "",
          });
        },
        updateUserRoles: (context, event) => {
          if (!context.userIdToUpdateRoles) {
            throw new Error("userIdToUpdateRoles is undefined");
          }

          return API.updateUserRoles(event.roles, context.userIdToUpdateRoles);
        },
      },

      actions: {
        clearSelectedUser: assign({
          userIdToSuspend: (_) => undefined,
          usernameToSuspend: (_) => undefined,
          userIdToDelete: (_) => undefined,
          usernameToDelete: (_) => undefined,
          userIdToActivate: (_) => undefined,
          usernameToActivate: (_) => undefined,
          userIdToResetPassword: (_) => undefined,
          userIdToUpdateRoles: (_) => undefined,
        }),
        assignUsers: assign({
          users: (_, event) => event.data.users,
          count: (_, event) => event.data.count,
        }),
        assignFilter: assign({
          filter: (_, event) => event.query,
        }),
        assignGetUsersError: assign({
          getUsersError: (_, event) => event.data,
        }),
        assignUserToSuspend: assign({
          userIdToSuspend: (_, event) => event.userId,
          usernameToSuspend: (_, event) => event.username,
        }),
        assignUserToDelete: assign({
          userIdToDelete: (_, event) => event.userId,
          usernameToDelete: (_, event) => event.username,
        }),
        assignUserToActivate: assign({
          userIdToActivate: (_, event) => event.userId,
          usernameToActivate: (_, event) => event.username,
        }),
        assignUserIdToResetPassword: assign({
          userIdToResetPassword: (_, event) => event.userId,
        }),
        assignUserIdToUpdateRoles: assign({
          userIdToUpdateRoles: (_, event) => event.userId,
        }),
        clearGetUsersError: assign((context: UsersContext) => ({
          ...context,
          getUsersError: undefined,
        })),
        assignSuspendUserError: assign({
          suspendUserError: (_, event) => event.data,
        }),
        assignDeleteUserError: assign({
          deleteUserError: (_, event) => event.data,
        }),
        assignActivateUserError: assign({
          activateUserError: (_, event) => event.data,
        }),
        assignResetUserPasswordError: assign({
          resetUserPasswordError: (_, event) => event.data,
        }),
        assignUpdateRolesError: assign({
          updateUserRolesError: (_, event) => event.data,
        }),
        clearUsers: assign((context: UsersContext) => ({
          ...context,
          users: undefined,
          count: undefined,
        })),
        clearSuspendUserError: assign({
          suspendUserError: (_) => undefined,
        }),
        clearDeleteUserError: assign({
          deleteUserError: (_) => undefined,
        }),
        clearActivateUserError: assign({
          activateUserError: (_) => undefined,
        }),
        clearResetUserPasswordError: assign({
          resetUserPasswordError: (_) => undefined,
        }),
        clearUpdateUserRolesError: assign({
          updateUserRolesError: (_) => undefined,
        }),
        displaySuspendSuccess: () => {
          displaySuccess(Language.suspendUserSuccess);
        },
        displaySuspendedErrorMessage: (context) => {
          const message = getErrorMessage(
            context.suspendUserError,
            Language.suspendUserError,
          );
          displayError(message);
        },
        displayDeleteSuccess: () => {
          displaySuccess(Language.deleteUserSuccess);
        },
        displayDeleteErrorMessage: (context) => {
          const message = getErrorMessage(
            context.deleteUserError,
            Language.deleteUserError,
          );
          displayError(message);
        },
        displayActivateSuccess: () => {
          displaySuccess(Language.activateUserSuccess);
        },
        displayActivatedErrorMessage: (context) => {
          const message = getErrorMessage(
            context.activateUserError,
            Language.activateUserError,
          );
          displayError(message);
        },
        displayResetPasswordSuccess: () => {
          displaySuccess(Language.resetUserPasswordSuccess);
        },
        displayResetPasswordErrorMessage: (context) => {
          const message = getErrorMessage(
            context.resetUserPasswordError,
            Language.resetUserPasswordError,
          );
          displayError(message);
        },
        displayUpdateRolesErrorMessage: (context) => {
          const message = getErrorMessage(
            context.updateUserRolesError,
            Language.updateUserRolesError,
          );
          displayError(message);
        },
        generateRandomPassword: assign({
          newUserPassword: (_) => generateRandomString(12),
        }),
        updateUserRolesInTheList: assign({
          users: ({ users }, event) => {
            if (!users) {
              return users;
            }

            return users.map((u) => {
              return u.id === event.data.id ? event.data : u;
            });
          },
        }),
        assignPaginationRef: assign({
          paginationRef: (context) =>
            spawn(
              paginationMachine.withContext(context.paginationContext),
              usersPaginationId,
            ),
        }),
        sendResetPage: send({ type: "RESET_PAGE" }, { to: usersPaginationId }),
      },
    },
  );
