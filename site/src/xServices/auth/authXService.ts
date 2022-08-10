import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"
import { displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  successProfileUpdate: "Updated settings.",
  successSecurityUpdate: "Updated password.",
  successRegenerateSSHKey: "SSH Key regenerated successfully",
}

export const checks = {
  readAllUsers: "readAllUsers",
  updateUsers: "updateUsers",
  createUser: "createUser",
  createTemplates: "createTemplates",
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
    action: "write",
  },
} as const

type Permissions = Record<keyof typeof permissionsToCheck, boolean>

export interface AuthContext {
  getUserError?: Error | unknown
  // The getMethods API call does not return an ApiError.
  // It can only error out in a generic fashion.
  getMethodsError?: Error | unknown
  authError?: Error | unknown
  updateProfileError?: Error | unknown
  updateSecurityError?: Error | unknown
  me?: TypesGen.User
  methods?: TypesGen.AuthMethods
  permissions?: Permissions
  checkPermissionsError?: Error | unknown
  // SSH
  sshKey?: TypesGen.GitSSHKey
  getSSHKeyError?: Error | unknown
  regenerateSSHKeyError?: Error | unknown
}

export type AuthEvent =
  | { type: "SIGN_OUT" }
  | { type: "SIGN_IN"; email: string; password: string }
  | { type: "UPDATE_PROFILE"; data: TypesGen.UpdateUserProfileRequest }
  | { type: "UPDATE_SECURITY"; data: TypesGen.UpdateUserPasswordRequest }
  | { type: "GET_SSH_KEY" }
  | { type: "REGENERATE_SSH_KEY" }
  | { type: "CONFIRM_REGENERATE_SSH_KEY" }
  | { type: "CANCEL_REGENERATE_SSH_KEY" }

export const authMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QEMCuAXAFgZXc9YAdLAJZQB2kA8hgMTYCSA4gHID6DLioADgPal0JPuW4gAHogBsATimEAzDIAcAFgUB2VTJ26ANCACeiAIwaADKsLKFtu-dsBfRwbRZc+IqQolyUBuS0ECJEvgBufADWXmTkAWL8gsKiSBKICubKhKpSyhoArAbGCGaqGoRSlVXVlfnOrhg4eATEsb7+gWAATl18XYQ8ADb4AGZ9ALatFPGpiSRCImKSCLLySmqa2ro6RaYFAEzWDscK9SBuTZ6EMOhCfgCqsN1BIYThUUQ3ALJgCQLzySW6Uy2VyBV2JXyUhMFRqcLqLnOjQ8LRudygj2e3V6-SGowm1zA6B+fySi1Sy32MnyhHyyksYK2ug0EJM1IURxO9jOFxRnyJ6IACt1xiRYKQRLAXpQ3uQItFCABjTBgRWRYVdUXi5LwWb-BYpUDLZRScygvKFIyIGQaQ4FHnI5r827tDVaiXkKXYvoDYboMaapUqtVusUe3W8fWAimIE1mnIW1l0rImOE1BENdxOwkuvw-LB8CBS4Iy94K75EzCFiMgOYGoEIZRUwgyVT5BQmfaW4omGxZGxcuwOrNXNHtfNVou0b24v0ByYVgtF0kA8lG2PN1vtzvd0zszmD06I3nZ7yUCABAa9EYkQahCB32j3QUAEQAggAVACibEFACUqAAMQYAAZL8V3rGMSjbGQKihLsISkOlaWHS4WjPSBLx4a9byIVAeAgfBXRwx8S1COUPkIE8rgwi9yCvPgbzvQh8MIoUSLABB3kVIiRAAbXMABdCDo3XaD8lgpCpAQq0EA0eTUL5KZzywjiWIIoi-EFDjpx6H08X9AlqPQ2JMPo7DGNw9S2OIyy7y4iieINAThL1MlDTScTJPg3dGxkfZFNPUy6OIWBMDeB8wFoJgvw-NhsGwAAJNgAGkvwATREtdPM7VQrE0XyTF7coB0PQKaOCy9xXCsc-ASxKUrAQxpXI+UiGMmIKDM0KaoFdp6sawwHIiJzkhcrKPOWIqkOscFZNTfYOXtY9HQqrqQuqnN0QGprdJxX18UDDrlO6zbaqgHahu43jyHGtzV0m0x9jyxQ5p7ExzBhUrB3Kkz1qqsLCEGPhkAgSAIsfP8vxilgvz-T8f3q1KMomht9g+8ocnMGQCtZDRZAPLkMyREc-pU+jNuB0HwcVEQb01S6-zAGBKC6TxaAAYTfFgOa-EC2ChmG4YR+KkuRzL7sgsT0bMQhzCUcxpMK20vsPBRieO2iAfCqmwYgJU6ZIBmksGpmWe6dmOaoFhgL-L4Behr9Yfh79ReStKJcjdy0Yx0Fsdx+b23MX7OvJnqgZBvXCC6ZmwFZzSLpN3ayNlNqqNWsnTsB3XwZj822e2pOrscm67q9h6GzMBQrDy7cZJ7DR1ZQlbSdDrOdcj3PY-jwuGt2mcDsMo6M7bjbs87-W87ji3e8G4a+FG-ihNRqCq5rtsO3r0wpG0ZvMzQ0eqtVVAunmQwIai5931d7Avw5+4-wYD9PdrKNsqmsoOQyBM3sQZ6pBDidDax9T7oHPqxBO2AQFnxaqnSimtKoU2gWA6ykDkHFxGqXZektRI5U-ooBkiZZIKFyHvEmB8gFH0VCfM+qDtroL2vpOcRkR6UKQdQ0B4CNL0I4Wfeei9brYPLlLPBjcCE-18m2KwGtWFa0CIwVgbAqD3A-CvaWWgYSEN-iUDIZpUxpiqBoQBZ52g0HQLAssoczFqM8k2WCW5N6+X2OYeShMTgyNbspUxdAB4GXnMpaxOD35-w0XLCRrJ0ZWH0QYqQRiW4UOVKqSI7RAJG1gOgTEXQLEUQVMdRJaoUlpIyU8Lo-CsGuWEbgykWR9jKChDoHItSZCK1UMoCE6MMiKCkBkaEqgTAkKKs4RE5BCxwDEAg9agTKnBJKNjGkRCezUjNB4ihJi-AzGmY9BAbZDjowWdvFQsIYlIUAedTJNjliqH2KyWQWRjm1FOX1LSIoww6guYgLG5ptEmHyGyI5MSVlKXOhOas7ztk6EIFSMEhVWz5TVkefeSk5EMSYveZiIyvx6S6GCquNIyhSS3o2cwgKgr-XMmpEgkVCAzhxY3PF+MfIQjyAi8hSLEEoqspSu8tKirZAZUrCEjcWUTLDhZVFdDbKopxT8+Z2iCgyGMeysVuFpUdlmr5Ok8gSVrTDptLlvwglbKKvkWVhVOyHG1ZnMevVcyJz7sUTZlcpEVH2bMuQiqyXhxzvrfVYLnG9hbKaHG3yChWHubEj1urx7U31rTcg9NxiM27jPA1jqoLPXMBahWAr5oaADd9dxkb24RxjdHZNBd+pFxxQoekhAip5tdamOpRbrUlr1tWqEdazDFUKm2ZQLbtaqq+t8zNHJtjju2AO9hNCUH6sIBirFtLoRywsI4iENaArxLZZ6p4vDZ1UppYayu+NNGrp3BCNswct2kt1egi+tLNArvlue4hH0p3EDvRAnhM6HWv29qvGV6r10kPfbun9Q6gO5tUFO6VLjIM9kzZG7x6A-UyA+iu5QL7ijOPyHCsq16rj5OSX4VJXR0nnKPVBJQ5RMjmEyFc2pvyoRSHaS4+QsSFBUg+j8pWgCY4QCNqqdEH4+BQPQPhL4oywXKCyPkWpmGcg2n2FJdpVIanqHyBoNDn0oTCpHmCqwjHZCtmksoZpO82myU3BOmzR5nBAA */
  createMachine(
    {
      context: {
        me: undefined,
        getUserError: undefined,
        authError: undefined,
        updateProfileError: undefined,
        methods: undefined,
        getMethodsError: undefined,
      },
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
            data: TypesGen.UserAuthorizationResponse
          }
          getSSHKey: {
            data: TypesGen.GitSSHKey
          }
          regenerateSSHKey: {
            data: TypesGen.GitSSHKey
          }
          hasFirstUser: {
            data: boolean
          }
        },
      },
      id: "authState",
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
            ssh: {
              initial: "idle",
              states: {
                idle: {
                  on: {
                    GET_SSH_KEY: {
                      target: "gettingSSHKey",
                    },
                  },
                },
                gettingSSHKey: {
                  entry: "clearGetSSHKeyError",
                  invoke: {
                    src: "getSSHKey",
                    onDone: [
                      {
                        actions: "assignSSHKey",
                        target: "loaded",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignGetSSHKeyError",
                        target: "idle",
                      },
                    ],
                  },
                },
                loaded: {
                  initial: "idle",
                  states: {
                    idle: {
                      on: {
                        REGENERATE_SSH_KEY: {
                          target: "confirmSSHKeyRegenerate",
                        },
                      },
                    },
                    confirmSSHKeyRegenerate: {
                      on: {
                        CANCEL_REGENERATE_SSH_KEY: {
                          target: "idle",
                        },
                        CONFIRM_REGENERATE_SSH_KEY: {
                          target: "regeneratingSSHKey",
                        },
                      },
                    },
                    regeneratingSSHKey: {
                      entry: "clearRegenerateSSHKeyError",
                      invoke: {
                        src: "regenerateSSHKey",
                        onDone: [
                          {
                            actions: ["assignSSHKey", "notifySuccessSSHKeyRegenerated"],
                            target: "idle",
                          },
                        ],
                        onError: [
                          {
                            actions: "assignRegenerateSSHKeyError",
                            target: "idle",
                          },
                        ],
                      },
                    },
                  },
                },
              },
            },
            security: {
              initial: "idle",
              states: {
                idle: {
                  initial: "noError",
                  states: {
                    noError: {},
                    error: {},
                  },
                  on: {
                    UPDATE_SECURITY: {
                      target: "updatingSecurity",
                    },
                  },
                },
                updatingSecurity: {
                  entry: "clearUpdateSecurityError",
                  invoke: {
                    src: "updateSecurity",
                    onDone: [
                      {
                        actions: "notifySuccessSecurityUpdate",
                        target: "#authState.signedIn.security.idle.noError",
                      },
                    ],
                    onError: [
                      {
                        actions: "assignUpdateSecurityError",
                        target: "#authState.signedIn.security.idle.error",
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
                cond: "dontHaveFirstUser",
                target: "redirectingToSetupPage",
              },
            ],
          },
        },
        redirectingToSetupPage: {
          entry: "redirectToSetupPage",
          type: "final",
        },
      },
    },
    {
      services: {
        signIn: async (_, event) => {
          return await API.login(event.email, event.password)
        },
        signOut: API.logout,
        getMe: API.getUser,
        getMethods: API.getAuthMethods,
        updateProfile: async (context, event) => {
          if (!context.me) {
            throw new Error("No current user found")
          }

          return API.updateProfile(context.me.id, event.data)
        },
        updateSecurity: async (context, event) => {
          if (!context.me) {
            throw new Error("No current user found")
          }

          return API.updateUserPassword(context.me.id, event.data)
        },
        checkPermissions: async (context) => {
          if (!context.me) {
            throw new Error("No current user found")
          }

          return API.checkUserPermissions(context.me.id, {
            checks: permissionsToCheck,
          })
        },
        // SSH
        getSSHKey: () => API.getUserSSHKey(),
        regenerateSSHKey: () => API.regenerateUserSSHKey(),
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
        clearUpdateSecurityError: assign({
          updateSecurityError: (_) => undefined,
        }),
        notifySuccessSecurityUpdate: () => {
          displaySuccess(Language.successSecurityUpdate)
        },
        assignUpdateSecurityError: assign({
          updateSecurityError: (_, event) => event.data,
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
        // SSH
        assignSSHKey: assign({
          sshKey: (_, event) => event.data,
        }),
        assignGetSSHKeyError: assign({
          getSSHKeyError: (_, event) => event.data,
        }),
        clearGetSSHKeyError: assign({
          getSSHKeyError: (_) => undefined,
        }),
        assignRegenerateSSHKeyError: assign({
          regenerateSSHKeyError: (_, event) => event.data,
        }),
        clearRegenerateSSHKeyError: assign({
          regenerateSSHKeyError: (_) => undefined,
        }),
        notifySuccessSSHKeyRegenerated: () => {
          displaySuccess(Language.successRegenerateSSHKey)
        },
      },
      guards: {
        dontHaveFirstUser: (_, event) => event.data,
      },
    },
  )
