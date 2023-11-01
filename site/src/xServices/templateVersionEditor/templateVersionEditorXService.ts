import {
  TemplateVersion,
  TemplateVersionVariable,
  VariableValue,
} from "api/typesGenerated";
import { assign, createMachine } from "xstate";
import * as API from "api/api";
import { PublishVersionData } from "pages/TemplateVersionEditorPage/types";

export interface TemplateVersionEditorMachineContext {
  orgId: string;
  templateId?: string;
  version?: TemplateVersion;
  publishingError?: unknown;
  lastSuccessfulPublishedVersion?: TemplateVersion;
  missingVariables?: TemplateVersionVariable[];
  missingVariableValues?: VariableValue[];
}

export const templateVersionEditorMachine = createMachine(
  {
    /** @xstate-layout N4IgpgJg5mDOIC5QBcwFsAOAbAhqgamAE6wCWA9gHYCiEpy5RAdKZfaTlqQF6tQDEASQByggCqCAggBlBALWoBtAAwBdRKAzkyyCpQ0gAHogC0AFgDsypgDYAnAFZlFpzefKbADgA0IAJ6IAIzKngDMTIF2FmYONoFW9s4Avkm+qJi4BMRkVLT0jCwQWGD8AMIAStSSYtQA+vjU5QDKggDywirqSCBaOnoGxggmoTYOTKEATIEOThNxDmajvgEIgTFjDhaeNpPTZpbRKWno2HhghCR6eQzMpEUlAAoAqgBCsk0AEp0GvexUA6Y5mNlKEzBNPJ5Ap4JjCbEt-EEJsozEw7MoHKEQhYJqFPBYbBYjiB0qcspdcnQboVivwACKCJoAWQZTVqzzeDI+tRekmEwka326v10-26gxMcM8TGcoRGc0CNjMyg8yyCkzsqOUkWcnjMEJhRJJmXO2SulIKOFgAGsHgBXABGXFgAAsHjgiDg0GBUCQyrzStRpGzXu8vmofto-voxaZAvFxnZ9mFonY7NswarVuCLEwLPjQlCPGsJodUsSTsaLjkaObmJabQ6na73Z7vdkyu0AGKCcqM4Mcz6CzSRkXR0CDOM5ta6ya4jHbZR2TNx1O5iHTHEWQLgzyGitnKtm-LMDCN0guviHqj8CBUMAsSgAN3IVvvp8d5+dl9NVCHPRH-QxggSrWOioSOHCdhzCEMzLuCGoFqMkJmGsgRynuGQHj+NbHkw75Nt+5KUPwxBEAUpIAGaMGgeFnhelBQFelB-sKgHjkEcbStESKbGYaZFoEy6LlKcJePE4IzDYEwYaSJpEdcBQAMY4JQilgFwDGCJQDxkVARBwLALy2qQWAQDed4Ps+r5MMpqnqUZJkQCxAGiuxQyhA4UpmLiLhePYaEjMuFgFqibjOPmqbKNJZZGlh8m1kwtrYOQOAQGI7rmZQ96sFZ95JVgKVpe6zl9K5RimFETAljioI4mmdgjBYy7QhsepWE4DVQShMmVthCnMIp+l4HwDmmZl2VPi+96DWAZyjU54ZCi5Y7lcBDgTNKeITGm+LYutNjNRMrV4uii7gRM+w9XF1b9UwADueCKV+DHzdI5BQLA-CSLStLck8gjSL90itAA4iVUYAggjg5o4UxJutkzbEFDgakionbImMKeVdZI3QlD3IE9I3GaZb0ffwLz-YDtS0u0SiLcOpUrYMvlMJJowwgqiZpsumPSoWnjIrEMTBTjcl47hBNEy9JMQGTn2lP6gb1I0LTtODo6QyYeKoidaZxBd0JxMuUnWCjFipiMITeeBYtMbdUvPVAr3vQrlTVHUDTNG0HQM-+TNa3ENi5vEnjQ6m20o4dgS2GC6L1W4upmHbfUJRR3rS4x2HjZZU1MOnhPOkxGtsatUkorqmxWLKFuC4JCIIHDwcuAj60uKCiwp-FuEF5nTE5zlee90X2GKIEXSMxDQHBDsuZah5cbBHYAWZk3uYzJEDVxlJdhdxLVIYGRmDIPg7ocI6cBMAVqV8Iy55kAxp9EOfxSfbeWW59ZsW40eB9HxgJ8z44AvrAK+hVb730vEAkBCBB7KVHJ0EuZVBhhVRAFTwvFI5TFXrVJg3l9Qll1CEGwe9f7kX-oA5+wDX7UhKE0agYhajMiaC0YQIN6iSHKFIN4nsZBPGoE0JBzNEAEgiDuWUippgW28qvdaG0rBrB3iEKKhIYr7h-hSXCh9yDHyfi-S+dwaSK2EAGIMzDWHsPwJw7h0heHSH4YIv2rFkFBEklVUI0ipgNSsPsVeJZg5RCxmhWYkRQikM0VSYe5Q4DkFtEQNSb8LKD2sjAZA0TYCxPiXAIRkNixjDQpCDwgTtz4hNk4WwDgFTKgxHxLcYSiSUHIBAOABhv7izIUQCMAcgImC3MHGUcog4gQOg3VMYx7DREWDEaEkJorHEwhonCVJWDsE4DwPgXSp5uQlCFPpUIkTInsBmBucYPARGiFFSYJYPAFnCUsgohiwCbM1j0gs8jqlgjRKmbcy4PIbTxEnI6BYYIODubdesdoPwujdB6L0Pp4BLW6ds7cGow6eVlOta2YIRkrG3MiTUGIsQ4jxASMFCV8KfkItWZ5pdBgeSlM4GYwtrZOF8ScuM4QkTgX2ASZUaZ6nzNkvbBKtk1IaSgFpHS719KwEMrLGlLihhKgZSUuuQIwg4tcZVYSWp4hSW2GEMluF8qFXSp0xFWzVrDEcNKSY6JDYzxxA4Q64R1ptxBEcw5RqqQzWGjLRyCrhGrEWGzDxVgwjeXWpCHwJzoSuqOkCKEnlwQkLUQs9pESCiO2Jo5eWgbIZwg2nHeeSIrAesOv0o5BIwR6lxLvNNQrU49wzk7Ji+agJeE5QSfyTgUYwjMKvLUGwbl2FGAU7qDberdz-jogBejqEtItS8ty20Y6JkWDCFw0zjkrCxuEFCCxLBxDzF6yd10Ol4QofOkBYCb4MTvrKqBVCQHtrcrKDU66pIlgWJ5HdiAcRKjQQsD1wRtoCvLOm4VWir3QJoY819q0wioo6mjLY2JFg4MVHg6Y4FZzbn7d6goUSYlxISQhicHiEJxCVCMewiotSrwKbYXYwQPAoTxCkFIQA */
    predictableActionArguments: true,
    id: "templateVersionEditor",
    schema: {
      context: {} as TemplateVersionEditorMachineContext,
      events: {} as
        | {
            type: "CREATED_VERSION";
            data: TemplateVersion;
          }
        | {
            type: "SET_MISSING_VARIABLE_VALUES";
            values: VariableValue[];
            fileId: string;
          }
        | { type: "CANCEL_MISSING_VARIABLE_VALUES" }
        | { type: "PUBLISH" }
        | ({ type: "CONFIRM_PUBLISH" } & PublishVersionData)
        | { type: "CANCEL_PUBLISH" },

      services: {} as {
        createBuild: {
          data: TemplateVersion;
        };
        fetchVersion: {
          data: TemplateVersion;
        };
        publishingVersion: {
          data: void;
        };
        loadMissingVariables: {
          data: TemplateVersionVariable[];
        };
      },
    },
    tsTypes: {} as import("./templateVersionEditorXService.typegen").Typegen0,
    initial: "idle",
    states: {
      idle: {
        on: {
          CREATED_VERSION: {
            actions: ["assignBuild"],
            target: "fetchingVersion",
          },
          PUBLISH: {
            target: "askPublishParameters",
          },
        },
      },

      askPublishParameters: {
        on: {
          CANCEL_PUBLISH: "idle",
          CONFIRM_PUBLISH: "publishingVersion",
        },
      },

      publishingVersion: {
        tags: "loading",
        entry: ["clearPublishingError", "clearLastSuccessfulPublishedVersion"],
        invoke: {
          id: "publishingVersion",
          src: "publishingVersion",

          onError: {
            actions: ["assignPublishingError"],
            target: "askPublishParameters",
          },

          onDone: {
            actions: ["assignLastSuccessfulPublishedVersion"],
            target: ["idle"],
          },
        },
      },
      creatingBuild: {
        tags: "loading",
        invoke: {
          id: "createBuild",
          src: "createBuild",
          onDone: {
            actions: "assignBuild",
            target: "fetchingVersion",
          },
        },
      },

      fetchingVersion: {
        tags: "loading",
        invoke: {
          id: "fetchVersion",
          src: "fetchVersion",

          onDone: [
            {
              actions: ["assignBuild"],
              target: "promptVariables",
              cond: "jobFailedWithMissingVariables",
            },
            {
              actions: ["assignBuild"],
              target: "idle",
            },
          ],
        },
      },

      promptVariables: {
        initial: "loadingMissingVariables",
        states: {
          loadingMissingVariables: {
            invoke: {
              src: "loadMissingVariables",
              onDone: {
                actions: "assignMissingVariables",
                target: "idle",
              },
            },
          },
          idle: {
            on: {
              SET_MISSING_VARIABLE_VALUES: {
                actions: "assignMissingVariableValues",
                target: "#templateVersionEditor.creatingBuild",
              },
              CANCEL_MISSING_VARIABLE_VALUES: {
                target: "#templateVersionEditor.idle",
              },
            },
          },
        },
      },
    },
  },
  {
    actions: {
      assignBuild: assign({
        version: (_, event) => event.data,
      }),
      assignLastSuccessfulPublishedVersion: assign({
        lastSuccessfulPublishedVersion: (ctx) => ctx.version,
        version: () => undefined,
      }),
      assignPublishingError: assign({
        publishingError: (_, event) => event.data,
      }),
      clearPublishingError: assign({ publishingError: (_) => undefined }),
      clearLastSuccessfulPublishedVersion: assign({
        lastSuccessfulPublishedVersion: (_) => undefined,
      }),
      assignMissingVariables: assign({
        missingVariables: (_, event) => event.data,
      }),
      assignMissingVariableValues: assign({
        missingVariableValues: (_, event) => event.values,
      }),
    },
    services: {
      createBuild: async (ctx, event) => {
        if (ctx.version?.job.status === "running") {
          await API.cancelTemplateVersionBuild(ctx.version.id);
        }
        return API.createTemplateVersion(ctx.orgId, {
          provisioner: "terraform",
          storage_method: "file",
          tags: {},
          template_id: ctx.templateId,
          file_id: event.fileId,
          user_variable_values: ctx.missingVariableValues,
        });
      },
      fetchVersion: (ctx) => {
        if (!ctx.version) {
          throw new Error("template version must be set");
        }
        return API.getTemplateVersion(ctx.version.id);
      },
      publishingVersion: async (
        { version, templateId },
        { name, message, isActiveVersion },
      ) => {
        if (!version) {
          throw new Error("Version is not set");
        }
        if (!templateId) {
          throw new Error("Template is not set");
        }
        const haveChanges =
          name !== version.name || message !== version.message;
        await Promise.all([
          haveChanges
            ? API.patchTemplateVersion(version.id, { name, message })
            : Promise.resolve(),
          isActiveVersion
            ? API.updateActiveTemplateVersion(templateId, {
                id: version.id,
              })
            : Promise.resolve(),
        ]);
      },
      loadMissingVariables: ({ version }) => {
        if (!version) {
          throw new Error("Version is not set");
        }
        const variables = API.getTemplateVersionVariables(version.id);
        return variables;
      },
    },
    guards: {
      jobFailedWithMissingVariables: (_, { data }) => {
        return data.job.error_code === "REQUIRED_TEMPLATE_VARIABLES";
      },
    },
  },
);
