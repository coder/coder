import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"
import { displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  successProfileUpdate: "Updated settings.",
}

export const checks = {
  readAllUsers: "readAllUsers",
  updateUsers: "updateUsers",
  createUser: "createUser",
  createTemplates: "createTemplates",
  deleteTemplates: "deleteTemplates",
  viewAuditLog: "viewAuditLog",
  viewDeploymentValues: "viewDeploymentValues",
  createGroup: "createGroup",
  viewUpdateCheck: "viewUpdateCheck",
  viewGitAuthConfig: "viewGitAuthConfig",
  viewDeploymentStats: "viewDeploymentStats",
  editWorkspaceProxies: "editWorkspaceProxies",
} as const

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
  [checks.viewGitAuthConfig]: {
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
} as const

export type Permissions = Record<keyof typeof permissionsToCheck, boolean>

export type AuthenticatedData = {
  user: TypesGen.User
  permissions: Permissions
}
export type UnauthenticatedData = {
  hasFirstUser: boolean
  authMethods: TypesGen.AuthMethods
}
export type AuthData = AuthenticatedData | UnauthenticatedData

export const isAuthenticated = (data?: AuthData): data is AuthenticatedData =>
  data !== undefined && "user" in data

const loadInitialAuthData = async (): Promise<AuthData> => {
  let authenticatedUser: TypesGen.User | undefined
  // User is injected by the Coder server into the HTML document.
  const userMeta = document.querySelector("meta[property=user]")
  if (userMeta) {
    const rawContent = userMeta.getAttribute("content")
    try {
      authenticatedUser = JSON.parse(rawContent as string) as TypesGen.User
    } catch (ex) {
      // Ignore this and fetch as normal!
    }
  }

  // If we have the user from the meta tag, we can skip this!
  if (!authenticatedUser) {
    authenticatedUser = await API.getAuthenticatedUser()
  }

  if (authenticatedUser) {
    const permissions = (await API.checkAuthorization({
      checks: permissionsToCheck,
    })) as Permissions
    return {
      user: authenticatedUser,
      permissions,
    }
  }

  const [hasFirstUser, authMethods] = await Promise.all([
    API.hasFirstUser(),
    API.getAuthMethods(),
  ])

  return {
    hasFirstUser,
    authMethods,
  }
}

const signIn = async (
  email: string,
  password: string,
): Promise<AuthenticatedData> => {
  await API.login(email, password)
  const [user, permissions] = await Promise.all([
    API.getAuthenticatedUser(),
    API.checkAuthorization({
      checks: permissionsToCheck,
    }),
  ])

  return {
    user: user as TypesGen.User,
    permissions: permissions as Permissions,
  }
}

const signOut = async () => {
  const [authMethods] = await Promise.all([
    API.getAuthMethods(), // Anticipate and load the auth methods
    API.logout(),
  ])

  return {
    hasFirstUser: true,
    authMethods,
  } as UnauthenticatedData
}
export interface AuthContext {
  error?: unknown
  updateProfileError?: unknown
  data?: AuthData
}

export type AuthEvent =
  | { type: "SIGN_OUT" }
  | { type: "SIGN_IN"; email: string; password: string }
  | { type: "UPDATE_PROFILE"; data: TypesGen.UpdateUserProfileRequest }

export const authMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QEMCuAXAFgZXc9YAdADYD2yEAlgHZQCS1l6lyxAghpgCL7IDEEUtSI0AbqQDWRNFlz4iZCjXqNmrDlh54EY0gGN8lIQG0ADAF0z5xKAAOpWEyPUbIAB6IAzABYAbIQBWUwB2AA4ARgAmAIAaEABPRHCQgE5AgF90uJkcPAIScipaBid1Ti1+QWFCXSlCHLl8xSKVUvZy3h1qcQNmEwtjcOskEHtHPpcRjwQffyCwqNiEpMiownCA32jM7M5GhULlErV2zV4BIRFuyWk9vIOlYtUWU+5O3V7nK2NI4bsHJxCVzTWaBEIRaJxRIIcI+byECEBHYgBr3AqPVonDRvPB8MAAJ3xpHxhFsxHwADNiQBbep3eTolrHF7YipdHqGfqWCyuMaAyagEF+MELSHLGHeYLBQh+ULBJFZFH0-KOKDCCAAeQwfGwdAA4gA5AD6dANVl5AImwKSnki8I2WyW0OSwTSCt2sjRqsYTwu1VqRG9DHNIz5VqmiClpkIplCARSnnlUKSqQyitRDO9R2oeMJxNJ5PQVPxtKD1BD-3GzmtCCjMbjCaT4vC4TjMdF7qVnszlDVkAYOv1xo1AFUACoV0aW6sjaEAWnCTemkUicplwV8m226eVgd76oYpKJFMoxBEEDPfBHAAUuGwxwBRI3XgBKGoAYnQADIPydhmegNCwSLoEcTLuEKThOum6OsiGYqvu-bUEepAnmehCoLYECGLQ17HqeYB+lc4h1PBe59hAh62Ph6GYdhzC4TRYDsvonLlgMPKhtOQKzpG8ZgUkATRIQq6LHBu6EN6SEoWhRB0ThUB4ahBG5kSJJkpSNJ0t2CEUVRTEYVhClKbJLGfFyf7cQK7iCcJolikBLbiTp5E+lAWroERNTXHU3oeZZVY8YKXjhL4ngypEwSeCkCaeAEnhRMEyYIKsraBJB0TePFAThN4zm5D2arKB5XkBpJ+7+UMFqBdZQqbIQvhZaEtpZRskreMlqWhA1pi+KEm4JgmWX5fs5VFbQJUEmpBaaSWY3UP5nGVvyNa2j4hBxb4KTymFEQpPFyWtlsIkbikpiRL4jUBBEngjWiehCCeUCoPiyhjpgYDvpQ+KwOgI6wASg6GiaZpLVONWuNCkSeAlCIw-DCMwwJKUw+sW4Koq1CkBAcCuGR1UrRGCALt4HXinOF2RGCpg06EpOmCkkSmHlO4uYy2ZtKyvAE+GwUIKYnUbNGiMi7drMFbp6oeTzAE2TCDMBAiTbOvt0admR83ZjLQVy1lVOrHKTpJFt3WXWb5tm+rElSZR1n-jr0zJPGStGwg2WEDFnspM1LbAcEd2FQeyHUcpZ7a7VSQui7yVbOFQSmC2trxllMUB5L0kh7JNQXmA4c1qFa6SjBDmRiB8eJ9EKQpykaeuRnBmUDnhBYw+eb4nnROLnF0Ho8loSmFbbM2-pofnuhU3Eh3fNd4rRe9+Kcpix6Et17bMkEYZ9HKCZBFT3LLYBIrhvJfKNfi6NWYTRge-LpB0bgosnUD-CouI7XhAPdQT0vW9H1fT9f0Abty4hDImc4Ui+E6jDKCzVX4w0yJkIAA */
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
            data: Awaited<ReturnType<typeof loadInitialAuthData>>
          }
          signIn: {
            data: Awaited<ReturnType<typeof signIn>>
          }
          updateProfile: {
            data: TypesGen.User
          }
          signOut: {
            data: Awaited<ReturnType<typeof signOut>>
          }
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
                actions: "assignError",
                target: "signedIn",
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
            throw new Error("Authenticated data is not loaded yet")
          }

          if (isAuthenticated(data)) {
            return API.updateProfile(data.user.id, event.data)
          }

          throw new Error("User not authenticated")
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
              throw new Error("No authentication data loaded")
            }

            return {
              ...context.data,
              user: event.data,
            }
          },
        }),
        assignUpdateProfileError: assign({
          updateProfileError: (_, event) => event.data,
        }),
        notifySuccessProfileUpdate: () => {
          displaySuccess(Language.successProfileUpdate)
        },
        clearUpdateProfileError: assign({
          updateProfileError: (_) => undefined,
        }),
        redirect: (_, _data) => {
          window.location.href = location.origin
        },
      },
      guards: {
        isAuthenticated: (_, { data }) => isAuthenticated(data),
        needSetup: (_, { data }) =>
          !isAuthenticated(data) && !data.hasFirstUser,
        hasRedirectUrl: (_, { data }) => Boolean(data),
      },
    },
  )
