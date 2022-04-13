import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"

export interface AuthContext {
  getUserError?: Error | unknown
  authError?: Error | unknown
  updateProfileError?: Error | unknown
  me?: Types.UserResponse
}

export type AuthEvent =
  | { type: "SIGN_OUT" }
  | { type: "SIGN_IN"; email: string; password: string }
  | { type: "UPDATE_PROFILE"; data: Types.UpdateProfileRequest }

export const authMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QEMCuAXAFgZXc9YAdLAJZQB2kA8hgMTYCSA4gHID6DLioADgPal0JPuW4gAHogCMABgCcAFkLyZAVilS5GgEwAOAMwA2ADQgAnogDsa5YcOXD2y7oWH92-QF9PptFlz4RKQUJORQDOS0ECJEoQBufADWQWTkEWL8gsKiSBKI7kraqpZSuh7aUvpy6vqmFghlcoT6qgb6GsUlctrevhg4eATEqaHhkWAAThN8E4Q8ADb4AGYzALbDFOm5mSRCImKSCLKKynJqGlpSekZ1iIaltvaOzq4FvSB+A4GEMOhCYQBVWCTKIxQjxJJEX4AWTAGQEu2yB0QrikhG0Mn0+l0ulUWJkhlatXMKKqynkCn0lOscnsUnenwCQ1+-ygQJBk2mswWyzWPzA6Fh8Ky+1yh202iacip2I8hjU9m0twQ1lUzSk9hkHlUCjU2kMDP6TJSFEgETm0yWJHmRFQPAg+FGAAVLdawKDKODyAlkoRGYMTZQIOaeK6bYQ7Q7WS6+FabQgIQBjR0iADaMgAusLEaLQIcNJY1YpZB5pQpJVJLMqMbpCKVulVCY5aZXDf4AxsgyGw7b7Y6wjG4+7OTM5ot0CsJut-d9gl3yBbY26I33oz2E96+Mm9uR01ntgid8iGlpCLpLNoFJXpYXHLplbr9IRi4oZDJdHJLIpim2vkM52aC6hkuNq0ACToACIAIIACoAKJsE6ABKVAAGIMAAMnB2ZHmKiDnjI6oOG4ihVM4yqfuiVS6hqEqFooBo+B8RodgBwaRIwrBsFQAIwThSJ4UcWLorSWKqDIlhGJWlhViSCCUaWNGOE4qiKHIv7Gp2ow0OgHqxJuvpzjp-G5nkCDuE+F5yLohiUqU2oeMqOq1moH4XrZL5eExM7-iMYQ6bQI7cuOk7rEZGAmTkeaIEUaruB+CjqFcRL6LJ9RFIRqUKFexQZQYEreEx5B8BAcBiD5gbUBFB4ilFZnqIYhBuNSajYoS1nKoWT4KAYqkeO0GKGOp3ksbOfljJFx5XPKdZ4hJ0qfu0ypDdoZ4ahe4klLiegaR2LKjOyEyTYJ5bKroGpnkSWiSbimiqLtY2muxi5DuCEDhsVcFTDMx3RUc0lnoWb4KJJhJag+F1ZZSqiDdZGKWA9vlPd2IGxO9RBBb9ZkFpYgPiTIINGOJSpya4jXiYo2WfvqEkSYjlXPcBr0kOjWP5lIeJ48DoPE8qMrNJY2VXKUzy6PTnaAS9y6Rv2UCDm6bP4QYhD0ZUqUltiUjKndTUfqWTaSvcCMje2j3zlLNqKw0rhEXY1FkfeclXFRN42Vqah4oYYsm3+DNbLwh4CX9ZSre0xH25+jv1AAtDNDgEgSqhJ7ZuKEuLc7adVAe1ce7iEFTl6FkUOIyFIChOeJyitGWpQEho9I+8aVu1gXIMw60uil+XcnR2rKvyvKydnBzrSFZ4QA */
  createMachine(
    {
      context: { me: undefined, getUserError: undefined, authError: undefined, updateProfileError: undefined },
      tsTypes: {} as import("./authXService.typegen").Typegen0,
      schema: {
        context: {} as AuthContext,
        events: {} as AuthEvent,
        services: {} as {
          getMe: {
            data: Types.UserResponse
          }
          signIn: {
            data: Types.LoginResponse
          }
          updateProfile: {
            data: Types.UserResponse
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
          invoke: {
            src: "signIn",
            id: "signIn",
            onDone: [
              {
                actions: "clearAuthError",
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
                target: "signedIn",
              },
            ],
            onError: [
              {
                actions: "assignGetUserError",
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
              states: {
                idle: {
                  states: {
                    noError: {},
                    error: {},
                  },
                },
                updatingProfile: {
                  invoke: {
                    src: "updateProfile",
                    onDone: [
                      {
                        actions: "assignMe",
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
              on: {
                UPDATE_PROFILE: {
                  target: ".updatingProfile",
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
                target: "signedOut",
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
        updateProfile: async (context, event) => {
          if (!context.me) {
            throw new Error("No current user found")
          }

          return API.updateProfile(context.me.id, event.data)
        },
      },
      actions: {
        assignMe: assign({
          me: (_, event) => event.data,
        }),
        unassignMe: assign((context: AuthContext) => ({
          ...context,
          me: undefined,
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
      },
    },
  )
