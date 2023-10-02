import { assign, createMachine } from "xstate";
import * as API from "api/api";
import * as TypesGen from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";

export const Language = {
  successProfileUpdate: "Updated settings.",
};

export const checks = {
  readAllUsers: "readAllUsers",
  updateUsers: "updateUsers",
  createUser: "createUser",
  createTemplates: "createTemplates",
  updateTemplates: "updateTemplates",
  deleteTemplates: "deleteTemplates",
  viewAuditLog: "viewAuditLog",
  viewDeploymentValues: "viewDeploymentValues",
  createGroup: "createGroup",
  viewUpdateCheck: "viewUpdateCheck",
  viewExternalAuthConfig: "viewExternalAuthConfig",
  viewDeploymentStats: "viewDeploymentStats",
  editWorkspaceProxies: "editWorkspaceProxies",
} as const;

export const permissionsToCheck = {
  [checks.readAllUsers]: {
    object: {
      resource_type: "user",
    },
    action: "read",
  },
  [checks.updateUsers]: {
    object: {
      resource_type: "user",
    },
    action: "update",
  },
  [checks.createUser]: {
    object: {
      resource_type: "user",
    },
    action: "create",
  },
  [checks.createTemplates]: {
    object: {
      resource_type: "template",
    },
    action: "update",
  },
  [checks.updateTemplates]: {
    object: {
      resource_type: "template",
    },
    action: "update",
  },
  [checks.deleteTemplates]: {
    object: {
      resource_type: "template",
    },
    action: "delete",
  },
  [checks.viewAuditLog]: {
    object: {
      resource_type: "audit_log",
    },
    action: "read",
  },
  [checks.viewDeploymentValues]: {
    object: {
      resource_type: "deployment_config",
    },
    action: "read",
  },
  [checks.createGroup]: {
    object: {
      resource_type: "group",
    },
    action: "create",
  },
  [checks.viewUpdateCheck]: {
    object: {
      resource_type: "deployment_config",
    },
    action: "read",
  },
  [checks.viewExternalAuthConfig]: {
    object: {
      resource_type: "deployment_config",
    },
    action: "read",
  },
  [checks.viewDeploymentStats]: {
    object: {
      resource_type: "deployment_stats",
    },
    action: "read",
  },
  [checks.editWorkspaceProxies]: {
    object: {
      resource_type: "workspace_proxy",
    },
    action: "create",
  },
} as const;

export type Permissions = Record<keyof typeof permissionsToCheck, boolean>;

export type AuthenticatedData = {
  user: TypesGen.User;
  permissions: Permissions;
};
export type UnauthenticatedData = {
  hasFirstUser: boolean;
  authMethods: TypesGen.AuthMethods;
};
export type AuthData = AuthenticatedData | UnauthenticatedData;

export const isAuthenticated = (data?: AuthData): data is AuthenticatedData =>
  data !== undefined && "user" in data;

const loadInitialAuthData = async (): Promise<AuthData> => {
  let authenticatedUser: TypesGen.User | undefined;
  // User is injected by the Coder server into the HTML document.
  const userMeta = document.querySelector("meta[property=user]");
  if (userMeta) {
    const rawContent = userMeta.getAttribute("content");
    try {
      authenticatedUser = JSON.parse(rawContent as string) as TypesGen.User;
    } catch (ex) {
      // Ignore this and fetch as normal!
    }
  }

  // If we have the user from the meta tag, we can skip this!
  if (!authenticatedUser) {
    authenticatedUser = await API.getAuthenticatedUser();
  }

  if (authenticatedUser) {
    const permissions = (await API.checkAuthorization({
      checks: permissionsToCheck,
    })) as Permissions;
    return {
      user: authenticatedUser,
      permissions,
    };
  }

  const [hasFirstUser, authMethods] = await Promise.all([
    API.hasFirstUser(),
    API.getAuthMethods(),
  ]);

  return {
    hasFirstUser,
    authMethods,
  };
};

const signIn = async (
  email: string,
  password: string,
): Promise<AuthenticatedData> => {
  await API.login(email, password);
  const [user, permissions] = await Promise.all([
    API.getAuthenticatedUser(),
    API.checkAuthorization({
      checks: permissionsToCheck,
    }),
  ]);

  return {
    user: user as TypesGen.User,
    permissions: permissions as Permissions,
  };
};

const signOut = async () => {
  const [authMethods] = await Promise.all([
    API.getAuthMethods(), // Anticipate and load the auth methods
    API.logout(),
  ]);

  return {
    hasFirstUser: true,
    authMethods,
  } as UnauthenticatedData;
};
export interface AuthContext {
  error?: unknown;
  updateProfileError?: unknown;
  data?: AuthData;
}

export type AuthEvent =
  | { type: "SIGN_OUT" }
  | { type: "SIGN_IN"; email: string; password: string }
  | { type: "UPDATE_PROFILE"; data: TypesGen.UpdateUserProfileRequest };

export const authMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QEMCuAXAFgZXc9YAdADYD2yEAlgHZQCS1l6lyxAghpgCL7IDEEUtSI0AbqQDWRNFlz4iZCjXqNmrDlh54EY0gGN8lIQG0ADAF0z5xKAAOpWEyPUbIAB6IAjAFZThACwATAAcAGwAzOGewZ7hoYH+ADQgAJ6Igaam3oSeGf7BcbnBAJyBAL5lyTI4eAQk5FS0DE7qnFr8gsKEulKE1XJ1io0qLextvDrU4gbMJhbGntZIIPaOsy7LHgg+fkFhkdGx8Ump6bEA7ITn56HnkaFFpRVVnAMKDcrNamOavAJCIimkmkr1q7yUTVULB+3AmuhmzisxkCSzsDicQlcWx2ARCESiMTiCWSaQQgUC5124UCxQKDxCT0qIH6YPqEJG3w0sLwfDAACc+aQ+YRbMR8AAzIUAWz6oPkbOGX2hXPak2mhjmlgsrlWGI2oGxvlx+wJR2JpzJ-lM4UIphKgXuj3KTJZ8scUGEEAA8hg+Ng6ABxAByAH06EGrDr0essV5TOdslloqZPKZ-Od-NESV4Hjb-OESqFvAyEudgs9mXK6u7GJD-l0ekQawxI8tdTHNl5ybtPKnPKFin3ivns2TU3mi1FrmFSuEK67q5QPZ9qLyBUKRWL0JK+TLm9RW2i1s5Y9tu4Q8ZksiFgmnR93PLbad5h6FMgm5y6q02l56GH7A1DL0AFUABVDxWaMT07BAogHQhgmCfwBxvbwiwKe87WyYIM1ibxPHzG5PxeWRWRrSAGBFQVxUoYgRAgOi+GAgAFLg2FAgBRENmIAJS9AAxOgABkOIg9toINdIi0fcJzn7dMshfXtR2ibx-EIUJkOKbwiVw4p52-QhyIgSjbGo2iiFQWwIEMWhmPMxjOkBcRegXH8PQo6gqNIGi6MIKybOYOyHLANV9A1A95m1NsoMxGDPGfHIiPCfxvDSwcCJUwsci0nT4j0gzSLdX9PO83zLOs2yoHsnyLLXQVhVFCVpVlIrFw8kyvLM2q-ICqqavKsKEU1MTYv1dwvESzxktS9LexOUlonyHKBzyilM30r82vc2soB9dB62c4EjN-fbRuPOLJNghK-HJckX2KFLimHFTSkCQhZIKUwByQotnRImpiuXWh9sO7ogV6GszsWKMLvGrY4n8dSbn8bTfFyXx01e8JsLU580unG5CsB9rdtB-kGs3ZrdxOj0zuio89VPWTTHenHTEegpwm+nDwkwzSPuKIjEPOckkOJt5CD0IQaKgVA+WUUDMDAfjKD5WB0GA2B+QA4MwwjBnILh09b1CHIOdTN8Uoe+9pve0WhZKHHNJRiomWoUgIDgVw3NhpmYIAWlCFS3w04cCwyDmKWpYjK22hUV1GFVeD9jsroD1MAhxule1zfNimDi1DnU0IYgyUoblTBIJbIkrvQwVOJIm2CE2CK5olKCIskQrLvovfCkOua14kQmugd2hhG8u5uC78FKHRCO4cPzQvSUCI4LxpUpEMiNNQjH0nPKn+GvDiM3KW52dcO+vmLQSNuEvzRC0uQtDZIPnbSu68rj9PAiCKuNaKOslMw31HJEbIA5874SvOEbSH9aZ-i6iFboDEwC-3igXM28RfB2kCH9I4o4gg2kflaeSalyShH3ltEmn9OplQsqgvyHsOLrj5Bgq669tIfTki7RSGVXpBBWtpXSmYcIIOMqZFBlA0GEApkKDhzcuHZFkvJSkc1PCYUzgRVaojojnAkXXKRPUKqBWUANCyijsQPGyEPO0s0lKZSLtlHRIj8obUMcDPaDcYrGxgvPG0dwaTHAIimfI-M2Z6WmlA8RNDJbS2oLLeWitlaq3VprbW7DfH+yumlPwj83zBBvKXYI3hbaiyuDSMsj00Lpk0m7MoQA */
  createMachine(
    {
      id: "authState",
      predictableActionArguments: true,
      tsTypes: {} as import("./authXService.typegen").Typegen0,
      schema: {
        context: {} as AuthContext,
        events: {} as AuthEvent,
        services: {} as {
          loadInitialAuthData: {
            data: Awaited<ReturnType<typeof loadInitialAuthData>>;
          };
          signIn: {
            data: Awaited<ReturnType<typeof signIn>>;
          };
          updateProfile: {
            data: TypesGen.User;
          };
          signOut: {
            data: Awaited<ReturnType<typeof signOut>>;
          };
        },
      },
      initial: "loadingInitialAuthData",
      states: {
        loadingInitialAuthData: {
          invoke: {
            src: "loadInitialAuthData",
            onDone: [
              {
                target: "signedIn",
                actions: ["assignData", "clearError"],
                cond: "isAuthenticated",
              },
              {
                target: "configuringTheFirstUser",
                actions: ["assignData", "clearError"],
                cond: "needSetup",
              },
              {
                target: "signedOut",
                actions: ["assignData", "clearError"],
              },
            ],
            onError: {
              target: "signedOut",
              actions: ["assignError"],
            },
          },
        },

        signedOut: {
          on: {
            SIGN_IN: {
              target: "signingIn",
            },
          },
        },

        signingIn: {
          entry: "clearError",
          invoke: {
            src: "signIn",
            id: "signIn",
            onDone: [
              {
                target: "signedIn",
                actions: "assignData",
              },
            ],
            onError: [
              {
                actions: "assignError",
                target: "signedOut",
              },
            ],
          },
        },

        signedIn: {
          type: "parallel",
          on: {
            SIGN_OUT: {
              target: "signingOut",
            },
          },
          states: {
            profile: {
              initial: "idle",
              states: {
                idle: {
                  initial: "noError",
                  states: {
                    noError: {},
                    error: {},
                  },
                  on: {
                    UPDATE_PROFILE: {
                      target: "updatingProfile",
                    },
                  },
                },
                updatingProfile: {
                  entry: "clearUpdateProfileError",
                  invoke: {
                    src: "updateProfile",
                    onDone: [
                      {
                        actions: ["updateUser", "notifySuccessProfileUpdate"],
                        target: "#authState.signedIn.profile.idle.noError",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignUpdateProfileError",
                        target: "#authState.signedIn.profile.idle.error",
                      },
                    ],
                  },
                },
              },
            },
          },
        },

        signingOut: {
          invoke: {
            src: "signOut",
            id: "signOut",
            onDone: [
              {
                actions: ["clearData", "clearError", "redirect"],
                cond: "hasRedirectUrl",
              },
              {
                actions: ["clearData", "clearError"],
                target: "signedOut",
              },
            ],
            onError: [
              {
                // The main way this is likely to fail is from the backend refusing
                // to talk to you because your token is already invalid
                actions: "assignError",
                target: "signedOut",
              },
            ],
          },
        },

        configuringTheFirstUser: {
          on: {
            SIGN_IN: {
              target: "signingIn",
            },
          },
        },
      },
    },
    {
      services: {
        loadInitialAuthData,
        signIn: (_, { email, password }) => signIn(email, password),
        signOut,
        updateProfile: async ({ data }, event) => {
          if (!data) {
            throw new Error("Authenticated data is not loaded yet");
          }

          if (isAuthenticated(data)) {
            return API.updateProfile(data.user.id, event.data);
          }

          throw new Error("User not authenticated");
        },
      },
      actions: {
        assignData: assign({
          data: (_, { data }) => data,
        }),
        clearData: assign({
          data: (_) => undefined,
        }),
        assignError: assign({
          error: (_, event) => event.data,
        }),
        clearError: assign({
          error: (_) => undefined,
        }),
        updateUser: assign({
          data: (context, event) => {
            if (!context.data) {
              throw new Error("No authentication data loaded");
            }

            return {
              ...context.data,
              user: event.data,
            };
          },
        }),
        assignUpdateProfileError: assign({
          updateProfileError: (_, event) => event.data,
        }),
        notifySuccessProfileUpdate: () => {
          displaySuccess(Language.successProfileUpdate);
        },
        clearUpdateProfileError: assign({
          updateProfileError: (_) => undefined,
        }),
        redirect: (_, _data) => {
          window.location.href = location.origin;
        },
      },
      guards: {
        isAuthenticated: (_, { data }) => isAuthenticated(data),
        needSetup: (_, { data }) =>
          !isAuthenticated(data) && !data.hasFirstUser,
        hasRedirectUrl: (_, { data }) => Boolean(data),
      },
    },
  );
