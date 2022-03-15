import { createMachine, interpret, assign } from "xstate"
import { UserResponse } from "../../api"
import * as API from "../../api"

export interface UserContext {
  error?: Error
  me?: UserResponse
}

export type UserEvent = { type: "SIGN_OUT" } | { type: "SIGN_IN"; email: string; password: string }

const userMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QFdZgE4GUAuBDbYAdLAJZQB2kA8stgMSYCSA4gHID6jrioADgPalsJfuR4gAHogDM0gJyEATAHYAbNIAscxdunKArKo0AaEAE9EG+YX0BGABz3FD6atsbbc5QF9vp1Bg4+ESkFCTkUIzkdBCiROEAbvwA1iFk5FHiAkIiYkiSiHIADITSOvbSRRqq2m7StqYWCIr2+ja2tvVV+vqaRVW+-mhYeATE6eGR0Rjo-OiEvAA2+ABmcwC24xSZ+dkkwqLiUgiytoQahna2Bo5FDeaI+optyo5y9nIOrQY+fiABI2ChBg2GEEQAqsMYnFCIkUkQQQBZMBZQT7XJHGRGQjKYrKDQfIqKKyKPSNRC2ImlRRFfS4jqqKrFOSDf7DIJjEFgqCQjB0GZzBbLbBrdCbJEo3Zog55UDHaT2EqaZSueSqHQ1Krk5plQhGeyvO7XIqqJysgEctIUSBRBgsDhUcEAFVROUO+WOBvs5zUKrUtlUdPk2tsKkIlMqqkDcn0rUq9nN7NGVvIkxo9FilFh5CSqS25HTrvR7rliCMGlK1Tkcij9lNF302qeqhx7iKXmcGg8tkTgWT+bTtH56Fm8yWqw2+cLUrdsoKCEDbSj7bUd2UoYJ2sULXDi4Mn3VlQ0vj+5H4EDg4gt-dClAg0740oxHseocIck0gf1jIu9yaXbOPR8XXXETVxBM-mvIFb0mHZH1nTEEDsQgiSMbQLlkZRum1LtvWuEC3BjaRYwMXtAU5MBQUmXl0CLGVEI0ZRCEVJ4sPKAxPGIkMqUpQ0rHxDQTQVMjLXzG05z2eiXwXao9Q3IosLsA1FBDMNFRqUNlEUVQDB0llIKTaCJgiB8QEk59SwQewHBQjwnAMFcCVUHDOhQ+QsPbWkPmskTkzoiz51JZjaRUIl3g4j9GweZoFF4xUtEZZ461UE9vCAA */
  createMachine(
    {
  tsTypes: {} as import("./userXService.typegen").Typegen0,
  schema: {
    context: {} as UserContext,
    events: {} as UserEvent,
    services: {} as {
      getMe: {
        data: API.UserResponse
      }
      signIn: {
        data: API.LoginResponse | undefined
      }
      signOut: {
        data: void
      }
    },
  },
  id: "userState",
  initial: "gettingUser",
  states: {
    signedOut: {
      on: {
        SIGN_IN: {
          target: "#userState.signingIn",
        },
      },
    },
    signingIn: {
      invoke: {
        src: "signIn",
        id: "signIn",
        onDone: [
          {
            target: "#userState.gettingUser",
          },
        ],
        onError: [
          {
            actions: "assignError",
            target: "#userState.signedOut",
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
            actions: "assignMe",
            target: "#userState.signedIn",
          },
        ],
        onError: [
          {
            actions: "assignError",
            target: "#userState.signedOut",
          },
        ],
      },
      tags: "loading",
    },
    signedIn: {
      on: {
        SIGN_OUT: {
          target: "#userState.signingOut",
        },
      },
    },
    signingOut: {
      invoke: {
        src: "signOut",
        id: "signOut",
        onDone: [
          {
            actions: "unassignMe",
            target: "#userState.signedOut",
          },
        ],
        onError: [
          {
            actions: "assignError",
            target: "#userState.signedIn",
          },
        ],
      },
      tags: "loading",
    },
  },
},
    {
      services: {
        signIn: async (_, event: UserEvent) => {
          return await API.login(event.email, event.password)
        },
        signOut: API.logout,
        getMe: API.getUser,
      },
      actions: {
        assignMe: assign({
          me: (_, event) => event.data,
        }),
        unassignMe: assign({
          me: () => undefined,
        }),
        assignError: assign({
          error: (_, event) => event.data,
        }),
      },
    },
  )

export const userService = interpret(userMachine).start()
