import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"
import { displaySuccess } from "../../components/Snackbar"

export const Language = {
  successProfileUpdate: "Updated preferences.",
}
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
  /** @xstate-layout N4IgpgJg5mDOIC5QEMCuAXAFgZXc9YAdLAJZQB2kA8hgMTYCSA4gHID6DLioADgPal0JPuW4gAHogCMABgCcAFkLyZAVilS5GgEwAOAMwA2ADQgAnogDsa5YcOXD2y7oWH92-QF9PptFlz4RKQUJORQDOS0ECJEoQBufADWQWTkEWL8gsKiSBKI7kraqpZSuh7aUvpy6vqmFghlcoT6qgb6GsUlctrevhg4eATEqaHhkWAAThN8E4Q8ADb4AGYzALbDFOm5mSRCImKSCLKKynJqGlpSekZ1iIaltvaOzq4FvSB+A4GEMOhCYQBVWCTKIxQjxJJEX4AWTAGQEu2yB0QrikhG0Mn0+l0ulUWJkhlatXMKKqynkCn0lOscnsUnenwCQ1+-ygQJBk2mswWyzWPzA6Fh8Ky+1yh202iacip2I8hjU9m0twQ1lUzSk9hkHlUCjU2kMDP6TJSFEgETm0yWJHmsQgNtoAIACgARACCABUAKJsR0AJSoADEGAAZT3CxGi0CHGTKmSG-yDE2UCDmniW61EVA8CD4UaO9P26KUcHkBLJQiMxMbZOpguZ7O5sL5vhWm0ICEAY1zIgA2jIALrhvY5KPSKSWNWKWQeaUKSXj5UY3SEUrdKqExy08fxr5DYI18gWlsZwhZnOs5utsC0TkzOaLdArCbrSvffdmw9p48208Ni919tSz4Lthz7QdtgRYdkQaLRCF0SxtAUcdpQnRxdGVXV9EIKdFBkGRdDkSxFGKHdjWrD96GYdgqABd0hyRMVpCxdFaSxVQZEsIxx0sSxlUI9Eql1DUJQnRQ5FIqt91GGh0FBYsIXLfcZPoyM8gQdwsIQuRdEMSlSm1DxlR1Zc1AIhDdJwrwfA+I1JJGMIZJvKY7x5R8+SUjAVJHNSijVdwCIUdQriJfReJJBAihkZpLAUJDikigwJW8azyD4CA4DEV891SahPIgkVvMOdRDEINxqTUbFCW05UJywhQDFUNd2gxQxxOsrKk1GLZeEghjRyOfUoqkPEOOlQj2mVVrtDgjUEPYkpcT0CTvhZUZ2QmLzoLnZVdA1OCiS0TjcU0VRluy00U0-OtwTtIhUs9ZyNvyiNCrHHi4InPCFE4wktQwvbQtiloWu0jFLDOpMPyPK8bp-W8np6groI0d74PYmRvqMdilXC1wSvYxRYsIwbrHB9rbLfHLLuhk8SFuzbGKOYasLRr6fux5UZWi2KCclXarL6BNKYu2tv3rc88zrBn+pxTTGsqULp2xKRlRO0qCJnDdJXuMnBd3SHqa-K9pbUgBaRDlVNzjyTwjpOM+1QDXJoXzoPE3DlN+rLakVwba1dpvvYjQIeraS8sRl7oPcQgicQicihxGQfaM9jlFaWdSgJDR6Wd-X3cQZdY8DhPdCThRLY8dUOPsBwDE4wjdGSzwgA */
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
        notifySuccessProfileUpdate: () => {
          displaySuccess(Language.successProfileUpdate)
        },
        clearUpdateProfileError: assign({
          updateProfileError: (_) => undefined,
        }),
      },
    },
  )
