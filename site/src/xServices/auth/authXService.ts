import { assign, createMachine } from "xstate"
import * as API from "../../api"
import * as Types from "../../api/types"

export interface AuthContext {
  getUserError?: Error | unknown
  authError?: Error | unknown
  me?: Types.UserResponse
}

export type AuthEvent = { type: "SIGN_OUT" } | { type: "SIGN_IN"; email: string; password: string }

export const authMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QFdZgE4GUAuBDbYAdLAJZQB2kA8stgMSYCSA4gHID6jrioADgPalsJfuR4gAHogDM0gJyEATAHYAbNIAscxdunKArKo0AaEAE9EG+YX0BGABz3FD6atsbbc5QF9vp1Bg4+ESkFCTkUIzkdBCiROEAbvwA1iFk5FHiAkIiYkiSiHIADITSOvbSRRqq2m7StqYWCIr2+ja2tvVV+vqaRVW+-mhYeATE6eGR0Rjo-OiEvAA2+ABmcwC24xSZ+dkkwqLiUghlyoRF+opayq4X+sqK+o2Il6qEcq5y+hrfjt+qgxAARGwUIMGwwgiAFVhjE4oREikiOCALJgLKCfa5I4yIyEZTFZQaezFK5lPTPBC2IqKUqKC4EjqqKrFOSA4FBMbgyFQGEYOgzOYLZbYNboTao9G7TEHPKgY7SewlTQ3dRyVQ6GpVSmKMqEIz2ZRKjrKIqqJzs4actIUSBRBgsDhUKEAFQxOUO+WOhvshA0ahualsqnu8kpthUhGplVUIa+rUq9ktgVGNvIkxo9FilAR5CSqS25Ez7qxnvliCMGlK1Tk6vN5p+T3ML0Ub2U7iKXmcGg8tmTILGoXTEUzAvQs3mS1WG0LxelHrlBQQIbasc7aiKtnbV3sOpaUZXBjkwd1A0B5H4EDg4g5qcL1FoJdlOIQdlpH2qhmJzJ+DWbCB7WxSmUIl2wJM0CSTPwgStO8h0mHY+BlbEvReICaSMbQflkU0fkpHtfS3MC3C+aR9ENfR+2tMEwAhSY+XQJ8UPLACziVS5TXKAxPDI8MaSjIojSqPQezNRUqLg9I7UXPZn1Q5dqn1CMNEEi4HAecNIyVGoIweVQDH0tloNvUF4JHR951LRdvQcc4PCcAx12-fDOnOeRTU7C4SXsPtjNg4ImLLJddUIdiVBpOQKJ4psmh0ASQOPRUiI8RRfF8IA */
  createMachine(
    {
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
        },
      },
      context: {
        me: undefined,
        getUserError: undefined,
        authError: undefined,
      },
      id: "authState",
      initial: "gettingUser",
      states: {
        signedOut: {
          on: {
            SIGN_IN: {
              target: "#authState.signingIn",
            },
          },
        },
        signingIn: {
          invoke: {
            src: "signIn",
            id: "signIn",
            onDone: [
              {
                target: "#authState.gettingUser",
                actions: "clearAuthError",
              },
            ],
            onError: [
              {
                actions: "assignAuthError",
                target: "#authState.signedOut",
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
                target: "#authState.signedIn",
              },
            ],
            onError: [
              {
                actions: "assignGetUserError",
                target: "#authState.signedOut",
              },
            ],
          },
          tags: "loading",
        },
        signedIn: {
          on: {
            SIGN_OUT: {
              target: "#authState.signingOut",
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
                target: "#authState.signedOut",
              },
            ],
            onError: [
              {
                actions: "assignAuthError",
                target: "#authState.signedIn",
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
      },
    },
  )
