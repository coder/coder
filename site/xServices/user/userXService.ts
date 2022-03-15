import { createMachine, interpret, assign } from "xstate"
import { UserResponse } from "../../api"
import * as API from "../../api"

export interface UserContext {
  error?: Error
  me?: UserResponse
}

export type UserEvent = { type: "SIGN_OUT" } | { type: "SIGN_IN"; email: string; password: string }

const userMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QFdZgE4GUAuBDbYAdLAJZQB2kA8stgMSYCSA4gHID6jrioADgPalsJfuR4gAHogAcAJkIBGAJxKArAoDMqpRoAsAdn0bZAGhABPRBulLCs2XLUaNABk0vVAX09nUGHPhEpBQk5FCM5HQQokShAG78ANZBZOQR4gJCImJIklYu8voFctKqGgoAbPraZpYIFdIudm66SroVeqoOFd6+aFh4BMSpoeGRGOj86IS8ADb4AGZTALbDFOm5mSTCouJSCHryunpKLrrHurL6yrWIZRWEFQpuJRr6ci76vSB+A4FrlAgEUIJAgszAdAAogARRgAFQygm22T2iAqSgeCme1zKXSUCmOt3qskxBPUBP0bUaum+vwCQ2CgOBkGRYSiMRB5ASyUILOwAFV+oisrtcvtpO1CNZdAp9LoPIdVBUiaoPIoye53tJZQTaf16SkKJAIgwWBwqPyEZskTscqB9uV9IptdI5Zp1KVqkTnqpCDpPm9WspjNY9f5BobyKMaPRopROdzIzHhcjRfbELJTs15a62mp5R4ibJVLpCDYVApVNdLsoHGG-gyRmEY3QJlMZvNsEt0KtGcnrSK7Xl6hUHi5tRV7LICy50d61QobHKMWS3IvvD4QOR+BA4OI6RGAdRaCnbaj6k0XEpZHoio0tG05fPfQVKq4NPj0RUCvWDQDRhsfA2iiYoZk6KiqK6zhVKqzymBYdxaIQBjLlcug2AS0i-oejLGuQIJgmAp4gemCBVtI6oBtqlZKtIyoIcSFFYro6hnMoLGtNh-y4UC+F8qMxFpsOlK2Bo35ysWWp6AoRKTk0zHkjehiyD+m4HtxqR4YJQ77KoJaUUY1F6Q09F1Mc8gKTmJzHFhan6jhTZQP2QGDuejROrK7Qfm8s5vEWFSlsWj5FLKrpeHZ4aBNp54aH6ahQWJ1RuG4bREsYhCvh0V6GCW0gaBunhAA */
  createMachine(
    {
      tsTypes: {} as import("./userXService.typegen").Typegen0,
      schema: {
        context: {} as UserContext,
        events: {} as UserEvent,
        services: {} as {
          getMe: {
            data: API.UserResponse
          },
          signIn: {
            data: API.LoginResponse | undefined
          },
          signOut: {
            data: void
          }
        }
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
          tags: "loading",
          invoke: {
            src: "signIn",
            id: "signIn",
            onDone: "#userState.gettingUser",
            onError: {
              actions: "assignError",
              target: "#userState.signedOut",
            },
          },
        },
        gettingUser: {
          tags: "loading",
          invoke: {
            src: "getMe",
            id: "getMe",
            onDone: {
              target: "signedIn",
              actions: "assignMe"
            },
            onError: {
              actions: "assignError"
            }
          }
        },
        signedIn: {
          on: {
            SIGN_OUT: {
              target: "#userState.signingOut",
            },
          },
        },
        signingOut: {
          tags: "loading",
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
                actions: ["assignError"],
                target: "#userState.signedOut",
              },
            ],
          },
        },
      },
    },
    {
      services: {
        signIn: async (context: UserContext, event: UserEvent) => {
          if (event.type === 'SIGN_IN') {
            return await API.login(event.email, event.password)
          }
        }, 
        signOut: API.logout,
        getMe: API.getUser
      },
      actions: {
        assignMe: assign({
          me: (_, event) => event.data
        }),
        unassignMe: assign({
          me: () => undefined,
        }),
        assignError: assign({
          error: (_, event) => event.data,
        }),
      }
    },
  )

export const userService = interpret(userMachine).start()
