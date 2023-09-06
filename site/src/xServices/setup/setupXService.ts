import * as API from "api/api";
import * as TypesGen from "api/typesGenerated";
import { assign, createMachine } from "xstate";

export interface SetupContext {
  error?: unknown;
  firstUser?: TypesGen.CreateFirstUserRequest;
}

export type SetupEvent = {
  type: "CREATE_FIRST_USER";
  firstUser: TypesGen.CreateFirstUserRequest;
};

export const setupMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QGUwBcCuAHZaCGaYAdAJYQA2YAxAMIBKAogIIAqDA+gGICSdyL7AKrIGdRKCwB7WCTQlJAO3EgAHogDMAViKaAnPoDsABgBMRgCy6jpkwBoQAT0QBGI+qIA2N0ePOAHJqa-qYAviH2qJg4+IREAMYATmAEJApQnCQJsGiCsGAJVBCKxKkAbpIA1sSJyYQZWTl5CcpSMnKKymoI6ibuzmZGuiYG6kYemn4eHvZOCObOzjom-X7qHsZGmuq6mmER6Ni4BNVJKWn12bn5VPkJkglEWOQEAGb3ALbxp3WZl00t0lk8iUSFUGl07g8-XMxlWmmsWnUMxcUyIzg86hhPgWvg8JjC4RACkkEDgykihxiJQoYABbWBnUQflcRAMZlc6jWwwMfmRCDM2ksfgMzl0Bisbj85j8exAFOixy+tVS6V+jXydKBHVBXQAtDsiOyeVp-OoFrpnHyzboiKZ1CMDCNzEY-H4xbL5UdYi81VcEjRvpBNe0QaAuoEbZzpfaRh4rFM+eYTOZbcsTBD0b0bB6DgrCMGGTrELrzYajM5jUFVubLY4NIsvKM2eMtKZNOMCSEgA */
  createMachine(
    {
      id: "SetupState",
      predictableActionArguments: true,
      tsTypes: {} as import("./setupXService.typegen").Typegen0,
      schema: {
        context: {} as SetupContext,
        events: {} as SetupEvent,
        services: {} as {
          createFirstUser: {
            data: TypesGen.CreateFirstUserResponse;
          };
        },
      },
      initial: "idle",
      states: {
        idle: {
          on: {
            CREATE_FIRST_USER: {
              actions: "assignFirstUserData",
              target: "creatingFirstUser",
            },
          },
        },
        creatingFirstUser: {
          entry: "clearError",
          invoke: {
            src: "createFirstUser",
            id: "createFirstUser",
            onDone: [
              {
                actions: "onCreateFirstUser",
                target: "firstUserCreated",
              },
            ],
            onError: {
              actions: "assignError",
              target: "idle",
            },
          },
          tags: "loading",
        },
        firstUserCreated: {
          tags: "loading",
          type: "final",
        },
      },
    },
    {
      services: {
        createFirstUser: (_, event) => API.createFirstUser(event.firstUser),
      },
      actions: {
        assignFirstUserData: assign({
          firstUser: (_, event) => event.firstUser,
        }),
        assignError: assign({
          error: (_, event) => event.data,
        }),
        clearError: assign({
          error: (_) => undefined,
        }),
      },
    },
  );
