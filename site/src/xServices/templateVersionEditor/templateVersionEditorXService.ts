import {
  ProvisionerJobLog,
  ProvisionerJobStatus,
  TemplateVersion,
  TemplateVersionVariable,
  UploadResponse,
  VariableValue,
  WorkspaceResource,
} from "api/typesGenerated";
import { assign, createMachine } from "xstate";
import * as API from "api/api";
import { FileTree, traverse } from "utils/filetree";
import { isAllowedFile } from "utils/templateVersion";
import { TarReader, TarWriter } from "utils/tar";
import { PublishVersionData } from "pages/TemplateVersionEditorPage/types";

export interface TemplateVersionEditorMachineContext {
  orgId: string;
  templateId?: string;
  fileTree?: FileTree;
  uploadResponse?: UploadResponse;
  version?: TemplateVersion;
  resources?: WorkspaceResource[];
  buildLogs?: ProvisionerJobLog[];
  tarReader?: TarReader;
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
        | { type: "INITIALIZE"; tarReader: TarReader }
        | {
            type: "CREATE_VERSION";
            fileTree: FileTree;
            templateId: string;
          }
        | { type: "CANCEL_VERSION" }
        | { type: "SET_MISSING_VARIABLE_VALUES"; values: VariableValue[] }
        | { type: "CANCEL_MISSING_VARIABLE_VALUES" }
        | { type: "ADD_BUILD_LOG"; log: ProvisionerJobLog }
        | { type: "BUILD_DONE" }
        | { type: "PUBLISH" }
        | ({ type: "CONFIRM_PUBLISH" } & PublishVersionData)
        | { type: "CANCEL_PUBLISH" },

      services: {} as {
        uploadTar: {
          data: UploadResponse;
        };
        createBuild: {
          data: TemplateVersion;
        };
        cancelBuild: {
          data: void;
        };
        fetchVersion: {
          data: TemplateVersion;
        };
        getResources: {
          data: WorkspaceResource[];
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
    initial: "initializing",
    states: {
      initializing: {
        on: {
          INITIALIZE: {
            actions: ["assignTarReader"],
            target: "idle",
          },
        },
      },

      idle: {
        on: {
          CREATE_VERSION: {
            actions: ["assignCreateBuild"],
            target: "cancelingInProgressBuild",
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

      cancelingInProgressBuild: {
        tags: "loading",
        invoke: {
          id: "cancelBuild",
          src: "cancelBuild",
          onDone: {
            target: "uploadTar",
          },
        },
      },

      uploadTar: {
        tags: "loading",
        invoke: {
          id: "uploadTar",
          src: "uploadTar",
          onDone: {
            target: "creatingBuild",
            actions: "assignUploadResponse",
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
            target: "watchingBuildLogs",
          },
        },
      },

      watchingBuildLogs: {
        tags: "loading",
        invoke: {
          id: "watchBuildLogs",
          src: "watchBuildLogs",
        },
        on: {
          ADD_BUILD_LOG: {
            actions: "addBuildLog",
          },
          BUILD_DONE: "fetchingVersion",
          CANCEL_VERSION: {
            target: "cancelingInProgressBuild",
          },
          CREATE_VERSION: {
            actions: ["assignCreateBuild"],
            target: "uploadTar",
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
              target: "fetchResources",
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

      fetchResources: {
        tags: "loading",
        invoke: {
          id: "getResources",
          src: "getResources",
          onDone: {
            actions: ["assignResources"],
            target: "idle",
          },
        },
      },
    },
  },
  {
    actions: {
      assignCreateBuild: assign({
        fileTree: (_, event) => event.fileTree,
        templateId: (_, event) => event.templateId,
        buildLogs: (_, _1) => [],
        resources: (_, _1) => [],
      }),
      assignResources: assign({
        resources: (_, event) => event.data,
      }),
      assignUploadResponse: assign({
        uploadResponse: (_, event) => event.data,
      }),
      assignBuild: assign({
        version: (_, event) => event.data,
      }),
      assignLastSuccessfulPublishedVersion: assign({
        lastSuccessfulPublishedVersion: (ctx) => ctx.version,
        version: () => undefined,
      }),
      addBuildLog: assign({
        buildLogs: (context, event) => {
          const previousLogs = context.buildLogs ?? [];
          return [...previousLogs, event.log];
        },
        // Instead of periodically fetching the version,
        // we just assume the state is running after the first log.
        //
        // The machine fetches the version after the log stream ends anyways!
        version: (context) => {
          if (!context.version || context.buildLogs?.length !== 0) {
            return context.version;
          }
          return {
            ...context.version,
            job: {
              ...context.version.job,
              status: "running" as ProvisionerJobStatus,
            },
          };
        },
      }),
      assignTarReader: assign({
        tarReader: (_, { tarReader }) => tarReader,
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
      uploadTar: async ({ fileTree, tarReader }) => {
        if (!fileTree) {
          throw new Error("file tree must to be set");
        }
        if (!tarReader) {
          throw new Error("tar reader must to be set");
        }
        const tar = new TarWriter();

        // Add previous non editable files
        for (const file of tarReader.fileInfo) {
          if (!isAllowedFile(file.name)) {
            if (file.type === "5") {
              tar.addFolder(file.name, {
                mode: file.mode, // https://github.com/beatgammit/tar-js/blob/master/lib/tar.js#L42
                mtime: file.mtime,
                user: file.user,
                group: file.group,
              });
            } else {
              tar.addFile(
                file.name,
                tarReader.getTextFile(file.name) as string,
                {
                  mode: file.mode, // https://github.com/beatgammit/tar-js/blob/master/lib/tar.js#L42
                  mtime: file.mtime,
                  user: file.user,
                  group: file.group,
                },
              );
            }
          }
        }
        // Add the editable files
        traverse(fileTree, (content, _filename, fullPath) => {
          // When a file is deleted. Don't add it to the tar.
          if (content === undefined) {
            return;
          }

          if (typeof content === "string") {
            tar.addFile(fullPath, content);
            return;
          }

          tar.addFolder(fullPath);
        });
        const blob = (await tar.write()) as Blob;
        return API.uploadFile(new File([blob], "template.tar"));
      },
      createBuild: (ctx) => {
        if (!ctx.uploadResponse) {
          throw new Error("no upload response");
        }
        return API.createTemplateVersion(ctx.orgId, {
          provisioner: "terraform",
          storage_method: "file",
          tags: {},
          template_id: ctx.templateId,
          file_id: ctx.uploadResponse.hash,
          user_variable_values: ctx.missingVariableValues,
        });
      },
      fetchVersion: (ctx) => {
        if (!ctx.version) {
          throw new Error("template version must be set");
        }
        return API.getTemplateVersion(ctx.version.id);
      },
      watchBuildLogs:
        ({ version }) =>
        async (callback) => {
          if (!version) {
            throw new Error("version must be set");
          }

          const socket = API.watchBuildLogsByTemplateVersionId(version.id, {
            onMessage: (log) => {
              callback({ type: "ADD_BUILD_LOG", log });
            },
            onDone: () => {
              callback({ type: "BUILD_DONE" });
            },
            onError: (error) => {
              console.error(error);
            },
          });

          return () => {
            socket.close();
          };
        },
      getResources: (ctx) => {
        if (!ctx.version) {
          throw new Error("template version must be set");
        }
        return API.getTemplateVersionResources(ctx.version.id);
      },
      cancelBuild: async (ctx) => {
        if (!ctx.version) {
          return;
        }
        if (ctx.version.job.status === "running") {
          await API.cancelTemplateVersionBuild(ctx.version.id);
        }
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
