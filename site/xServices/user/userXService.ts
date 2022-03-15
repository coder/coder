import { createMachine, interpret, assign } from "xstate"
import { UserResponse } from "../../api"
import * as API from "../../api"

export interface UserContext {
  error?: Error
  me?: UserResponse
}

export type UserEvent = { type: "SIGN_OUT" } | { type: "SIGN_IN"; email: string; password: string }

const userMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QFdZgE4GUAuBDbYAdLAJZQB2kA8stgMSYCSA4gHID6jrioADgPalsJfuR4gAHogDs0gIyF5ADmkBmAJyyALACY5W6QBoQAT0QBWdecIAGOzZ2qlANhtznlgL6fjqDDnwiUgoScihGcjoIUSJQgDd+AGsgsnII8QEhETEkSUQddQUC5xUHG2cK1TlVYzMELX1CCoqlGyUdA2dVCu9fNCw8AmJU0PDIjHR+dEJeABt8ADMpgFthinTczJJhUXEpBAKi9RLpMuaqmtMZHR1Cc3sdczln6UstXpA-AcDCGGxhMIAVX6URihHiSSIfwAsmAMoJttk9tdVIQNOVquV1IV5LVEKpXrZ7HIdEoya8bKpzB8vgEhn8AVBgRg6BMpjN5tgluhVjC4ZsETscqB9tJHGj1BjVFicXI8fUlNZ7G4lAStFpzEp9DT+nSUhRIBEGCwOFRAQAVeFZXa5UXmazOZRilzHUnyyWEbFe9QNX2yHQ6-yDfXkUY0ejRSjg8gJZJrcjhq2Im0ilESqUyuS4q4HDxE+zSLSUmy6czOQPfIbBUNhcOs9CTaZzRYreOJgXW4V5BBi1Ho5yY5zYrNynNye35uxyHGqVQGaTeHwgcj8CBwcS04Px6i0JNC5EISVKQgdbFKdRkmxqVQ6eVPVFFux6KnucwFCt6+OjDZ8QVI22ILo8oNNIhANC0HQ2OoNyyNSS6bj8DKjMy6B7v+qb1M4twDhqYpyEoWhOCU8oaAo7gQYWOhQQRH5btWhpdls+4AYebQnloZ4Xq0163mO7R3MqHHmFU+hKLRPzVmGu4dsmXb7Oqx56CoBKKic2LukU9z2GeA74XBfRBoEaEpt2+HylmnrerOpYqI6AaLkAA */
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
      initial: "signedOut",
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
            onDone: {
              target: "#userState.gettingUser",
            },

            onError: {
              actions: "assignError",
              target: "#userState.signedOut",
            },
          },
          tags: "loading",
        },
        gettingUser: {
          invoke: {
            src: "getMe",
            id: "getMe",
            onDone: {
              actions: "assignMe",
              target: "#userState.signedIn",
            },
            onError: {
              actions: "assignError",
              target: "#userState.signedOut",
            },
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
            onDone: {
              actions: "unassignMe",
              target: "#userState.signedOut",
            },
            onError: {
              actions: "assignError",
              target: "#userState.signedIn",
            },
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
