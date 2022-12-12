import { TemplateExample } from "api/typesGenerated"
import { createMachine } from "xstate"

interface CreateTemplateContext {
  exampleId?: string
  error?: unknown
}

export const createTemplateMachine = createMachine({
  id: "createTemplate",
  predictableActionArguments: true,
  schema: {
    context: {} as CreateTemplateContext,
    events: {} as { type: "CREATE"; data: any },
    services: {} as {
      loadingStarterTemplate: {
        data: TemplateExample
      }
    },
  },
  tsTypes: {} as import("./createTemplateXService.typegen").Typegen0,
  states: {
    starting: {
      always: [
        { target: "loadingStarterTemplate", cond: "isExampleProvided" },
        { target: "idle" },
      ],
    },
    loadingStarterTemplate: {
      invoke: {
        src: "loadStarterTemplate",
        onDone: {
          target: "idle",
          actions: ["assignStarterTemplate"],
        },
        onError: {
          target: "idle",
          actions: ["assignError"],
        },
      },
    },
    idle: {
      on: {
        CREATE: "creating",
      },
    },
    creating: {
      invoke: {
        src: "createTemplate",
        onDone: {
          target: "created",
          actions: ["onCreated"],
        },
        onError: {
          target: "idle",
          actions: ["assignError"],
        },
      },
    },
    created: {
      type: "final",
    },
  },
})
