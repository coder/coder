import { getExperiments } from "api/api"
import { Experiment } from "api/typesGenerated"
import { createMachine, assign } from "xstate"

export interface ExperimentsContext {
  experiments?: Experiment[]
  getExperimentsError?: Error | unknown
}

export const experimentsMachine = createMachine(
  {
    id: "experimentsState",
    predictableActionArguments: true,
    tsTypes: {} as import("./experimentsMachine.typegen").Typegen0,
    schema: {
      context: {} as ExperimentsContext,
      services: {} as {
        getExperiments: {
          data: Experiment[]
        }
      },
    },
    initial: "gettingExperiments",
    states: {
      gettingExperiments: {
        invoke: {
          src: "getExperiments",
          id: "getExperiments",
          onDone: [
            {
              actions: ["assignExperiments", "clearGetExperimentsError"],
              target: "#experimentsState.success",
            },
          ],
          onError: [
            {
              actions: ["assignGetExperimentsError", "clearExperiments"],
              target: "#experimentsState.failure",
            },
          ],
        },
      },
      success: {
        type: "final",
      },
      failure: {
        type: "final",
      },
    },
  },
  {
    services: {
      getExperiments: getExperiments,
    },
    actions: {
      assignExperiments: assign({
        experiments: (_, event) => event.data,
      }),
      clearExperiments: assign((context: ExperimentsContext) => ({
        ...context,
        experiments: undefined,
      })),
      assignGetExperimentsError: assign({
        getExperimentsError: (_, event) => event.data,
      }),
      clearGetExperimentsError: assign((context: ExperimentsContext) => ({
        ...context,
        getExperimentsError: undefined,
      })),
    },
  },
)
