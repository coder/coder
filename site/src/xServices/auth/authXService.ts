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
  viewDeploymentConfig: "viewDeploymentConfig",
  createGroup: "createGroup",
  viewUpdateCheck: "viewUpdateCheck",
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
  [checks.viewDeploymentConfig]: {
    object: {
      resource_type: "deployment_flags",
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
      resource_type: "update_check",
    },
    action: "read",
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
  const authenticatedUser = await API.getAuthenticatedUser()

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
  // Get app hostname so we can see if we need to log out of app URLs.
  // We need to load this before we log out of the API as this is an
  // authenticated endpoint.
  const appHost = await API.getApplicationsHost()
  const [authMethods] = await Promise.all([
    API.getAuthMethods(), // Antecipate and load the auth methods
    API.logout(),
  ])

  // Logout the app URLs
  if (appHost.host !== "") {
    const { protocol, host } = window.location
    const redirect_uri = encodeURIComponent(`${protocol}//${host}/login`)
    // The path doesn't matter but we use /api because the dev server
    // proxies /api to the backend.
    const uri = `${protocol}//${appHost.host.replace(
      "*",
      "coder-logout",
    )}/api/logout?redirect_uri=${redirect_uri}`

    return {
      redirectUrl: uri,
    }
  }

  return {
    hasFirstUser: true,
    authMethods,
  } as UnauthenticatedData
}
export interface AuthContext {
  error?: Error | unknown
  updateProfileError?: Error | unknown
  data?: AuthData
}

export type AuthEvent =
  | { type: "SIGN_OUT" }
  | { type: "SIGN_IN"; email: string; password: string }
  | { type: "UPDATE_PROFILE"; data: TypesGen.UpdateUserProfileRequest }

export const authMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QEMCuAXAFgZXc9YAdLAJZQB2kA8hgMTYCSA4gHID6DLioADgPal0JPuW4gAHogBsATimEAzDIAcAFgUB2VTJ26ANCACeiAIwaADKsLKFtu-dsBfRwbRZc+IqQolyUBuS0ECJEvgBufADWXmTkAWL8gsKiSBKICubKhKpSyhoArAbGCGaqGoRSlVXVlfnOrhg4eATEsb7+gWAATl18XYQ8ADb4AGZ9ALatFPGpiSRCImKSCLLySmqa2ro6RaYFAEzWDscK9SBuTZ6EMOhCfgCqsN1BIYThUUQ3ALJgCQLzySW6Uy2VyBV2JXyUhMFRqcLqLnOjQ8LRudygj2e3V6-SGowm1zA6B+fySi1Sy32MnyhHyyksYK2ug0EJM1IURxO9jOFxRnyJ6IACt1xiRYKQRLAXpQ3uQItFCABjTBgRWRYVdUXi5LwWb-BYpUDLZRScygvKFIyIGQaQ4FHnI5r827tDVaiXkKXYvoDYboMaapUqtVusUe3W8fWAimIE1mnIW1l0rImOE1BENdxOwkuvw-LB8CBS4Iy94K75EzCFiMgOYGoEIZRUwgyVT5BQmfaW4omGxZGxcuwOrNXNHtfNVou0b24v0ByYVgtF0kA8lG2PN1vtzvd0zszmD06I3nZ7yUCABAa9EYkQahCB32j3QUAEQAggAVACibEFACUqAAMQYAAZL8V3rGMSjbGQKihLsISkOlaWHS4WjPSBLx4a9byIVAeAgfBXRwx8S1COUPkIE8rgwi9yCvPgbzvQh8MIoUSLABB3kVIiRAAbXMABdCDo3XaD8lgpCpAQq0EA0eTUL5KZzywjiWIIoi-EFDjpx6H08X9AlqPQ2JMPo7DGNw9S2OIyy7y4iieINAThL1MlDTScTJPg3dGxkfZFNPUy6OIWBMDeB8wFoJgvw-NhsGwAAJNgAGkvwATREtdPM7VQrE0XyTF7coB0PQKaOCy9xXCsc-ASxKUrAQxpXI+UiGMmIKDM0KaoFdp6sawwHIiJzkhcrKPOWIqkOscFZNTfYOXtY9HQqrqQuqnN0QGprdJxX18UDDrlO6zbaqgHahu43jyHGtzV0m0x9jyxQ5p7ExzBhUrB3Kkz1qqsLCEGPhkAgSAIsfP8vxilgvz-T8f3q1KMomht9g+8ocnMGQCtZDRZAPLkMyREc-pU+jNuB0HwcVEQb01S6-zAGBKC6TxaAAYTfFgOa-EC2ChmG4YR+KkuRzL7sgsT0bMQhzCUcxpMK20vsPBRieO2iAfCqmwYgJU6ZIBmksGpmWe6dmOaoFhgL-L4Behr9Yfh79ReStKJcjdy0Yx0Fsdx+b23MX7OvJnqgZBvXCC6ZmwFZzSLpN3ayNlNqqNWsnTsB3XwZj822e2pOrscm67q9h6GzMBQrDy7cZJ7DR1ZQlbSdDrOdcj3PY-jwuGt2mcDsMo6M7bjbs87-W87ji3e8G4a+FG-ihNRqCq5rtsO3r0wpG0ZvMzQ0eqtVVAunmQwIai5931d7Avw5+4-wYD9PdrKNsqmsoOQyBM3sQZ6pBDidDax9T7oHPqxBO2AQFnxaqnSimtKoU2gWA6ykDkHFxGqXZektRI5U-ooBkiZZIKFyHvEmB8gFH0VCfM+qDtroL2vpOcRkR6UKQdQ0B4CNL0I4Wfeei9brYPLlLPBjcCE-18m2KwGtWFa0CIwVgbAqD3A-CvaWWgYSEN-iUDIZpUxpiqBoQBZ52g0HQLAssoczFqM8k2WCW5N6+X2OYeShMTgyNbspUxdAB4GXnMpaxOD35-w0XLCRrJ0ZWH0QYqQRiW4UOVKqSI7RAJG1gOgTEXQLEUQVMdRJaoUlpIyU8Lo-CsGuWEbg5YmhCD7B3nU-YA5lA6HVhCZxYjoRshyDvJQ+NVCAIAO7IABH4QCfQPwqlSV0dJmT6DMHYJwGx1Sm7Yx0I3SwziTBOLqWaLQdIpC2HyKoLZ5gESInIIWOAYgEHrUCZU4JJRsY0iIT2akZoPEUJMX4GY9zHoIDbIcdGLzt4qFhDEpCgDzqZKWYgVQ+xWSyCyOC2okK+paRFGGHUML-mmnNNorZbIwUxI+Upc6E5qzYq2LUuQwKSitnymrI8+8lJyIYkxe8zELlfj0l0bFVcaRlCklvRspzjGILZVZEgkVCAzj5Y3AV+MfIQjyEy8hLLxUWXZRfOVRVsiKqVhCRuhxtgaBVY3A5MgxX-XMmpCB7E7K-CCX8oq+RnnaIKJa+J6rrUSrvHykwHZZq+X2bScwYbzUSTUBCr1QUfWbSlX6p1lctlusKp2Q4JLY1hzOmixOfdii-MrlIiotKPpyCtdm8e1N9YJsdYWqCmyshyH9vigoVhkWxIre3CO1aDbkHpuMRm3cZ51tft7BtqhzAZoVga+aGhexuOOJmtalaO69qnj3fqRc+UKHpIQIq87S25GXZnMea69Y7qhPuswxVCptnKNsOQ7Z2xUhPYfCmYV-WBtLVOjkJqzUkP6TGldp10EX0IFynlcroRywsI4iEu6ArAdPVQmhKDa0yqg0m1e+NNFwZ3BCNswdkPvuIGB2tcqakuPlgR4h2MWzMgAxartwDeEoLtf1dB-rXVBoQyQljqHOFfq+q2v9jHG7mqUKqm55N-UuN4-NT6DGdD5C0Gp-Ypq31eL8HcsdFcG0yA+rB5QtHijOKOYoNWgD8nJNGUU6F2GxK9LlvSTIcLGn5C2ZUNpLj5CxIUFSD6XmuyDOGeiMZXQJlgCmTMkp2LGm1OUFCHQORGkyEVqoZQbTNly2-poaERy6kh0pYl5LrZpLNIy1l2Sm5dAkPRs4wz+NlDOGcEAA */
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
        redirect: (_, { data }) => {
          if (!("redirectUrl" in data)) {
            throw new Error(
              "Redirect only should be called with data.redirectUrl",
            )
          }

          window.location.replace(data.redirectUrl)
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
