import {
  getPreviousTemplateVersionByName,
  GetPreviousTemplateVersionByNameResponse,
  getTemplateByName,
  getTemplateVersionByName,
} from "api/api";
import { Template, TemplateVersion } from "api/typesGenerated";
import {
  getTemplateVersionFiles,
  TemplateVersionFiles,
} from "utils/templateVersion";
import { assign, createMachine } from "xstate";

export interface TemplateVersionMachineContext {
  orgId: string;
  templateName: string;
  versionName: string;
  template?: Template;
  currentVersion?: TemplateVersion;
  currentFiles?: TemplateVersionFiles;
  error?: unknown;
  // Get file diffs
  previousVersion?: TemplateVersion;
  previousFiles?: TemplateVersionFiles;
}

export const templateVersionMachine = createMachine(
  {
    predictableActionArguments: true,
    id: "templateVersion",
    schema: {
      context: {} as TemplateVersionMachineContext,
      services: {} as {
        loadVersions: {
          data: {
            currentVersion: GetPreviousTemplateVersionByNameResponse;
            previousVersion: GetPreviousTemplateVersionByNameResponse;
          };
        };
        loadTemplate: {
          data: {
            template: Template;
          };
        };
        loadFiles: {
          data: {
            currentFiles: TemplateVersionFiles;
            previousFiles: TemplateVersionFiles;
          };
        };
      },
    },
    tsTypes: {} as import("./templateVersionXService.typegen").Typegen0,
    initial: "initialInfo",
    states: {
      initialInfo: {
        type: "parallel",
        states: {
          versions: {
            initial: "loadingVersions",
            states: {
              loadingVersions: {
                invoke: {
                  src: "loadVersions",
                  onDone: [
                    {
                      actions: "assignVersions",
                      target: "success",
                    },
                  ],
                },
              },
              success: {
                type: "final",
              },
            },
          },
          template: {
            initial: "loadingTemplate",
            states: {
              loadingTemplate: {
                invoke: {
                  src: "loadTemplate",
                  onDone: [
                    {
                      actions: "assignTemplate",
                      target: "success",
                    },
                  ],
                },
              },
              success: {
                type: "final",
              },
            },
          },
        },
        onDone: {
          target: "loadingFiles",
        },
      },
      loadingFiles: {
        invoke: {
          src: "loadFiles",
          onDone: {
            target: "done.ok",
            actions: ["assignFiles"],
          },
          onError: {
            target: "done.error",
            actions: ["assignError"],
          },
        },
      },
      done: {
        states: {
          ok: { type: "final" },
          error: { type: "final" },
        },
      },
    },
  },
  {
    actions: {
      assignError: assign({
        error: (_, { data }) => data,
      }),
      assignTemplate: assign({
        template: (_, { data }) => data.template,
      }),
      assignVersions: assign({
        currentVersion: (_, { data }) => data.currentVersion,
        previousVersion: (_, { data }) => data.previousVersion,
      }),
      assignFiles: assign({
        currentFiles: (_, { data }) => data.currentFiles,
        previousFiles: (_, { data }) => data.previousFiles,
      }),
    },
    services: {
      loadVersions: async ({ orgId, templateName, versionName }) => {
        const [currentVersion, previousVersion] = await Promise.all([
          getTemplateVersionByName(orgId, templateName, versionName),
          getPreviousTemplateVersionByName(orgId, templateName, versionName),
        ]);

        return {
          currentVersion,
          previousVersion,
        };
      },
      loadTemplate: async ({ orgId, templateName }) => {
        const template = await getTemplateByName(orgId, templateName);

        return {
          template,
        };
      },
      loadFiles: async ({ currentVersion, previousVersion }) => {
        if (!currentVersion) {
          throw new Error("Version is not defined");
        }
        const loadFilesPromises: ReturnType<typeof getTemplateVersionFiles>[] =
          [];
        loadFilesPromises.push(getTemplateVersionFiles(currentVersion));
        if (previousVersion) {
          loadFilesPromises.push(getTemplateVersionFiles(previousVersion));
        }
        const [currentFiles, previousFiles] = await Promise.all(
          loadFilesPromises,
        );
        return {
          currentFiles,
          previousFiles,
        };
      },
    },
  },
);
