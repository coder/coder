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

export interface AuthContext {
  getUserError?: Error | unknown
  // The getMethods API call does not return an ApiError.
  // It can only error out in a generic fashion.
  getMethodsError?: Error | unknown
  authError?: Error | unknown
  updateProfileError?: Error | unknown
  me?: TypesGen.User
  methods?: TypesGen.AuthMethods
  permissions?: Permissions
  checkPermissionsError?: Error | unknown
}

export type AuthEvent =
  | { type: "SIGN_OUT" }
  | { type: "SIGN_IN"; email: string; password: string }
  | { type: "UPDATE_PROFILE"; data: TypesGen.UpdateUserProfileRequest }
  | { type: "GET_AUTH_METHODS" }

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
          getMe: {
            data: TypesGen.User
          }
          getMethods: {
            data: TypesGen.AuthMethods
          }
          signIn: {
            data: TypesGen.LoginWithPasswordResponse
          }
          updateProfile: {
            data: TypesGen.User
          }
          updateSecurity: {
            data: undefined
          }
          checkPermissions: {
            data: TypesGen.AuthorizationResponse
          }
          hasFirstUser: {
            data: boolean
          }
          signOut: {
            data:
              | {
                  redirectUrl: string
                }
              | undefined
          }
        },
      },
      context: {
        me: undefined,
        getUserError: undefined,
        authError: undefined,
        updateProfileError: undefined,
        methods: undefined,
        getMethodsError: undefined,
      },
      initial: "gettingUser",
      states: {
        signedOut: {
          on: {
            SIGN_IN: {
              target: "signingIn",
            },
          },
        },
        signingIn: {
          entry: "clearAuthError",
          invoke: {
            src: "signIn",
            id: "signIn",
            onDone: [
              {
                target: "gettingUser",
              },
            ],
            onError: [
              {
                actions: "assignAuthError",
                target: "signedOut",
              },
            ],
          },
          tags: "loading",
        },
        gettingUser: {
          entry: "clearGetUserError",
          invoke: {
            src: "getMe",
            id: "getMe",
            onDone: [
              {
                actions: "assignMe",
                target: "gettingPermissions",
              },
            ],
            onError: [
              {
                actions: "assignGetUserError",
                target: "checkingFirstUser",
              },
            ],
          },
          tags: "loading",
        },
        gettingPermissions: {
          entry: "clearGetPermissionsError",
          invoke: {
            src: "checkPermissions",
            id: "checkPermissions",
            onDone: [
              {
                actions: "assignPermissions",
                target: "signedIn",
              },
            ],
            onError: [
              {
                actions: "assignGetPermissionsError",
                target: "signedOut",
              },
            ],
          },
          tags: "loading",
        },
        gettingMethods: {
          invoke: {
            src: "getMethods",
            id: "getMethods",
            onDone: [
              {
                actions: ["assignMethods", "clearGetMethodsError"],
                target: "signedOut",
              },
            ],
            onError: [
              {
                actions: "assignGetMethodsError",
                target: "signedOut",
              },
            ],
          },
          tags: "loading",
        },
        signedIn: {
          type: "parallel",
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
                        actions: ["assignMe", "notifySuccessProfileUpdate"],
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
            methods: {
              initial: "idle",
              states: {
                idle: {
                  on: {
                    GET_AUTH_METHODS: {
                      target: "gettingMethods",
                    },
                  },
                },
                gettingMethods: {
                  entry: "clearGetMethodsError",
                  invoke: {
                    src: "getMethods",
                    onDone: [
                      {
                        actions: ["assignMethods", "clearGetMethodsError"],
                        target: "idle",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignGetMethodsError",
                        target: "idle",
                      },
                    ],
                  },
                },
              },
            },
          },
          on: {
            SIGN_OUT: {
              target: "signingOut",
            },
          },
        },
        signingOut: {
          invoke: {
            src: "signOut",
            id: "signOut",
            onDone: [
              {
                actions: ["unassignMe", "clearAuthError", "redirect"],
                cond: "hasRedirectUrl",
              },
              {
                actions: ["unassignMe", "clearAuthError"],
                target: "gettingMethods",
              },
            ],
            onError: [
              {
                actions: "assignAuthError",
                target: "signedIn",
              },
            ],
          },
          tags: "loading",
        },
        checkingFirstUser: {
          invoke: {
            src: "hasFirstUser",
            onDone: [
              {
                cond: "isTrue",
                target: "gettingMethods",
              },
              {
                target: "waitingForTheFirstUser",
              },
            ],
            onError: "signedOut",
          },
          tags: "loading",
        },
        waitingForTheFirstUser: {
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
        signIn: async (_, event) => {
          return await API.login(event.email, event.password)
        },
        signOut: async () => {
          // Get app hostname so we can see if we need to log out of app URLs.
          // We need to load this before we log out of the API as this is an
          // authenticated endpoint.
          const appHost = await API.getApplicationsHost()
          await API.logout()

          if (appHost.host !== "") {
            const { protocol, host } = window.location
            const redirect_uri = encodeURIComponent(
              `${protocol}//${host}/login`,
            )
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
        },
        getMe: API.getUser,
        getMethods: API.getAuthMethods,
        updateProfile: async (context, event) => {
          if (!context.me) {
            throw new Error("No current user found")
          }

          return API.updateProfile(context.me.id, event.data)
        },
        checkPermissions: async () => {
          return API.checkAuthorization({
            checks: permissionsToCheck,
          })
        },
        // First user
        hasFirstUser: () => API.hasFirstUser(),
      },
      actions: {
        assignMe: assign({
          me: (_, event) => event.data,
        }),
        unassignMe: assign((context: AuthContext) => ({
          ...context,
          me: undefined,
        })),
        assignMethods: assign({
          methods: (_, event) => event.data,
        }),
        assignGetMethodsError: assign({
          getMethodsError: (_, event) => event.data,
        }),
        clearGetMethodsError: assign((context: AuthContext) => ({
          ...context,
          getMethodsError: undefined,
        })),
        assignGetUserError: assign({
          getUserError: (_, event) => event.data,
        }),
        clearGetUserError: assign((context: AuthContext) => ({
          ...context,
          getUserError: undefined,
        })),
        assignAuthError: assign({
          authError: (_, event) => event.data,
        }),
        clearAuthError: assign((context: AuthContext) => ({
          ...context,
          authError: undefined,
        })),
        assignUpdateProfileError: assign({
          updateProfileError: (_, event) => event.data,
        }),
        notifySuccessProfileUpdate: () => {
          displaySuccess(Language.successProfileUpdate)
        },
        clearUpdateProfileError: assign({
          updateProfileError: (_) => undefined,
        }),
        assignPermissions: assign({
          // Setting event.data as Permissions to be more stricted. So we know
          // what permissions we asked for.
          permissions: (_, event) => event.data as Permissions,
        }),
        assignGetPermissionsError: assign({
          checkPermissionsError: (_, event) => event.data,
        }),
        clearGetPermissionsError: assign({
          checkPermissionsError: (_) => undefined,
        }),
        redirect: (_, { data }) => {
          // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- data can be undefined
          if (!data) {
            throw new Error(
              "Redirect only should be called with data.redirectUrl",
            )
          }

          window.location.replace(data.redirectUrl)
        },
      },
      guards: {
        isTrue: (_, event) => event.data,
        hasRedirectUrl: (_, { data }) => Boolean(data),
      },
    },
  )
