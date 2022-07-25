import { assign, createMachine } from "xstate"
import * as API from "../../api/api"
import * as TypesGen from "../../api/typesGenerated"
import { displayError, displaySuccess } from "../../components/GlobalSnackbar/utils"

export const Language = {
  successProfileUpdate: "Updated settings.",
  successSecurityUpdate: "Updated password.",
  successRegenerateSSHKey: "SSH Key regenerated successfully",
  errorRegenerateSSHKey: "Error on regenerate the SSH Key",
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

const sshState = {
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
            actions: ["assignSSHKey"],
            target: "#authState.signedIn.ssh.loaded",
          },
        ],
        onError: [
          {
            actions: "assignGetSSHKeyError",
            target: "#authState.signedIn.ssh.idle",
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
            CANCEL_REGENERATE_SSH_KEY: "idle",
            CONFIRM_REGENERATE_SSH_KEY: "regeneratingSSHKey",
          },
        },
        regeneratingSSHKey: {
          entry: "clearRegenerateSSHKeyError",
          invoke: {
            src: "regenerateSSHKey",
            onDone: [
              {
                actions: ["assignSSHKey", "notifySuccessSSHKeyRegenerated"],
                target: "#authState.signedIn.ssh.loaded.idle",
              },
            ],
            onError: [
              {
                actions: ["assignRegenerateSSHKeyError", "notifySSHKeyRegenerationError"],
                target: "#authState.signedIn.ssh.loaded.idle",
              },
            ],
          },
        },
      },
    },
  },
}

export const authMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QEMCuAXAFgZXc9YAdLAJZQB2kA8hgMTYCSA4gHID6DLioADgPal0JPuW4gAHogDsABgCshOQA4AzABY5ARgBMUgJxrtcgDQgAnok0zNSwkpkrNKvQDY3aqS6kBfb6bRYuPhEpBQk5FAM5LQQIkThAG58ANYhZORRYvyCwqJIEohyMlKE2mrFKo7aLq5SJuaILtq2mjZqrboy2s4+fiABOHgExOnhkdFgAE6TfJOEPAA2+ABmswC2IxSZ+dkkQiJikgiyCsrqWroGRqYWCHoq2oQyDpouXa1qGmq+-hiDwYQYOghBEAKqwKYxOKERIpIhAgCyYCyAj2uUOiF0mieTik3UqMhqSleN0QhgUOhUbzUKhk9jKch+-T+QWGQJBUHBkKmMzmixW60BYHQSJROQO+SOUmxVKUniUcjkUgVLjlpOOCqecj0LyKmmVeiUTIGrPhwo5SKwfAgsChlBh5CSqSFIuFmGt8B2qP2eVARxUeNKCo02k0cllTRc6qUalsCtU731DikvV+gSGZuBY0t7pttB5s3mS3Qq0mG0Rbo9YrREr9iHUMkIUnKHSklQVKnqtz0BkImjUbmeShcVmejL6Jozm0oECi8xmyxIC3iEGXtFBAAUACIAQQAKgBRNgbgBKVAAYgwADIH6s+jEIOV6Qgj1oxnTBkfq7qNlxyT4aC4saaPcVLGiyU6hDOc48AuS5EKgPAQPgYwbnBa6xPasLOpOAJQZAMHoQhSEoREaF8Iuy4ILCADGKEiAA2jIAC6d7opKiBPi+rRtB+-5fg0CDqM+YafG8+jWLI2jgemeHpAR5DzhR8GEIhyEcuRlFgPm0yFvyJaCrhwz4bOimwcpy6qSRGlEdRjp8HRPpMaxXrir6BSPvo3Fvu0zT8Zo6oDmoTb-p8cjaFcsjqDJ-zGfJpn0Mw7BUKCe5sbWHm6CU2jWKGnhaFSeLqv2jZKMOehOE49i0v+MWmtOYw0OgdrxPZzpQU16XuUcAC0-aPGGSgVQOTS4ko2jfjGhC0gB1WeDIBguHVkGjBETU6byRYCmW06da5NbdZiKalLl+p-k4XgTYJNhxuVNKGCB77fEy5DWnAYhGWkFDUBgXUPoqLiKOoxIqGVejNKoxXPCUMgjZ8zj6sORoThBclhBE2y8N67F1o+xIvhoejavqEX-gFgl6IGFWOC2bzWNqy0AuyYxcpMf0cQghjqn+JT6GJeJynIqrI2msWZhalY2uzuN9dojwpn+epDvqHjRkUL7KBDEljiLzKyXF32mUpWkwquRCvQeuls-t94c606s8c0A5C00UjqqDja5cUXQRXiDiMwb0FmURpuWQW1tY25D7242jsxn+bi6IFhh2K4qjKP+fsqAHX1B8bKkkGb0seR0gNx87idu4JtIwzoxTdELzbheOov1SZhEWcR6moURxdHCOgPhsBoNDRDKju84hCfHS4ZAXPqg59OCn58ufeFODQPD2DY-FUqGsASmdShgvKP67nClrwgQu2EPIPb2V4+CVNf7w5TOphs22en2LDVrb9Ns4w8j1coU8iafDKJVWQagDDqmOsOKBVhXDgweNJb+ppL59TcH2ZQw1E5jSurcYK8CHCODfE0B4vRfBAA */
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
          invoke: {
            src: "getMe",
            id: "getMe",
            onDone: [
              {
                actions: ["assignMe", "clearGetUserError"],
                target: "gettingPermissions",
              },
            ],
            onError: [
              {
                actions: "assignGetUserError",
                target: "gettingMethods",
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
                actions: ["assignPermissions"],
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
            ssh: sshState,
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
                        actions: ["notifySuccessSecurityUpdate"],
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
        notifySSHKeyRegenerationError: () => {
          displayError(Language.errorRegenerateSSHKey)
        },
      },
    },
  )
