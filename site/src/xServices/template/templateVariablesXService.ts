import {
  createTemplateVersion,
  getTemplateVersion,
  getTemplateVersionVariables,
  updateActiveTemplateVersion,
} from "api/api";
import {
  CreateTemplateVersionRequest,
  Template,
  TemplateVersion,
  TemplateVersionVariable,
} from "api/typesGenerated";
import { assign, createMachine } from "xstate";
import { delay } from "utils/delay";
import { Message } from "api/types";

type TemplateVariablesContext = {
  organizationId: string;

  template: Template;
  activeTemplateVersion?: TemplateVersion;
  templateVariables?: TemplateVersionVariable[];

  createTemplateVersionRequest?: CreateTemplateVersionRequest;
  newTemplateVersion?: TemplateVersion;

  getTemplateDataError?: unknown;
  updateTemplateError?: unknown;

  jobError?: TemplateVersion["job"]["error"];
};

type UpdateTemplateEvent = {
  type: "UPDATE_TEMPLATE_EVENT";
  request: CreateTemplateVersionRequest;
};

export const templateVariablesMachine = createMachine(
  {
    id: "templateVariablesState",
    predictableActionArguments: true,
    tsTypes: {} as import("./templateVariablesXService.typegen").Typegen0,
    schema: {
      context: {} as TemplateVariablesContext,
      events: {} as UpdateTemplateEvent,
      services: {} as {
        getActiveTemplateVersion: {
          data: TemplateVersion;
        };
        getTemplateVariables: {
          data: TemplateVersionVariable[];
        };
        createNewTemplateVersion: {
          data: TemplateVersion;
        };
        waitForJobToBeCompleted: {
          data: TemplateVersion;
        };
        updateTemplate: {
          data: Message;
        };
      },
    },
    initial: "gettingActiveTemplateVersion",
    states: {
      gettingActiveTemplateVersion: {
        entry: "clearGetTemplateDataError",
        invoke: {
          src: "getActiveTemplateVersion",
          onDone: [
            {
              actions: ["assignActiveTemplateVersion"],
              target: "gettingTemplateVariables",
            },
          ],
          onError: {
            actions: ["assignGetTemplateDataError"],
            target: "error",
          },
        },
      },
      gettingTemplateVariables: {
        entry: "clearGetTemplateDataError",
        invoke: {
          src: "getTemplateVariables",
          onDone: [
            {
              actions: ["assignTemplateVariables"],
              target: "fillingParams",
            },
          ],
          onError: {
            actions: ["assignGetTemplateDataError"],
            target: "error",
          },
        },
      },
      fillingParams: {
        on: {
          UPDATE_TEMPLATE_EVENT: {
            actions: ["assignCreateTemplateVersionRequest", "clearJobError"],
            target: "creatingTemplateVersion",
          },
        },
      },
      creatingTemplateVersion: {
        entry: "clearUpdateTemplateError",
        invoke: {
          src: "createNewTemplateVersion",
          onDone: {
            actions: ["assignNewTemplateVersion"],
            target: "waitingForJobToBeCompleted",
          },
          onError: {
            actions: ["assignGetTemplateDataError"],
            target: "fillingParams",
          },
        },
        tags: ["submitting"],
      },
      waitingForJobToBeCompleted: {
        invoke: {
          src: "waitForJobToBeCompleted",
          onDone: [
            {
              target: "fillingParams",
              cond: "hasJobError",
              actions: ["assignJobError"],
            },
            {
              actions: ["assignNewTemplateVersion"],
              target: "updatingTemplate",
            },
          ],
          onError: {
            actions: ["assignUpdateTemplateError"],
            target: "fillingParams",
          },
        },
        tags: ["submitting"],
      },
      updatingTemplate: {
        invoke: {
          src: "updateTemplate",
          onDone: {
            target: "updated",
            actions: ["onUpdateTemplate"],
          },
          onError: {
            actions: ["assignUpdateTemplateError"],
            target: "fillingParams",
          },
        },
        tags: ["submitting"],
      },
      updated: {
        entry: "onUpdateTemplate",
        type: "final",
      },
      error: {},
    },
  },
  {
    services: {
      getActiveTemplateVersion: ({ template }) => {
        return getTemplateVersion(template.active_version_id);
      },
      getTemplateVariables: ({ template }) => {
        return getTemplateVersionVariables(template.active_version_id);
      },
      createNewTemplateVersion: ({
        organizationId,
        createTemplateVersionRequest,
      }) => {
        if (!createTemplateVersionRequest) {
          throw new Error("Missing request body");
        }
        return createTemplateVersion(
          organizationId,
          createTemplateVersionRequest,
        );
      },
      waitForJobToBeCompleted: async ({ newTemplateVersion }) => {
        if (!newTemplateVersion) {
          throw new Error("Template version is undefined");
        }

        let status = newTemplateVersion.job.status;
        while (["pending", "running"].includes(status)) {
          newTemplateVersion = await getTemplateVersion(newTemplateVersion.id);
          status = newTemplateVersion.job.status;
          await delay(2_000);
        }
        return newTemplateVersion;
      },
      updateTemplate: ({ template, newTemplateVersion }) => {
        if (!newTemplateVersion) {
          throw new Error("New template version is undefined");
        }

        return updateActiveTemplateVersion(template.id, {
          id: newTemplateVersion.id,
        });
      },
    },
    actions: {
      assignActiveTemplateVersion: assign({
        activeTemplateVersion: (_, event) => event.data,
      }),
      assignTemplateVariables: assign({
        templateVariables: (_, event) => event.data,
      }),
      assignCreateTemplateVersionRequest: assign({
        createTemplateVersionRequest: (_, event) => event.request,
      }),
      assignNewTemplateVersion: assign({
        newTemplateVersion: (_, event) => event.data,
      }),
      assignGetTemplateDataError: assign({
        getTemplateDataError: (_, event) => event.data,
      }),
      clearGetTemplateDataError: assign({
        getTemplateDataError: (_) => undefined,
      }),
      assignUpdateTemplateError: assign({
        updateTemplateError: (_, event) => event.data,
      }),
      clearUpdateTemplateError: assign({
        updateTemplateError: (_) => undefined,
      }),
      assignJobError: assign({
        jobError: (_, event) => event.data.job.error,
      }),
      clearJobError: assign({
        jobError: (_) => undefined,
      }),
    },
    guards: {
      hasJobError: (_, { data }) => {
        return Boolean(data.job.error);
      },
    },
  },
);
