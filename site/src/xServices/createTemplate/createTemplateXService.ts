import {
  getTemplateExamples,
  createTemplateVersion,
  getTemplateVersion,
  createTemplate,
  getTemplateVersionSchema,
  uploadTemplateFile,
} from "api/api"
import {
  CreateTemplateRequest,
  ParameterSchema,
  Template,
  TemplateExample,
  TemplateVersion,
  UploadResponse,
} from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
import { assign, createMachine } from "xstate"

// for creating a new template:
// 1. upload template tar or use the example ID
// 2. create template version
// 3. wait for it to complete
// 4. if the job failed with the missing parameter error then:
//    a. prompt for params
//    b. create template version again with the same file hash
//    c. wait for it to complete
// 5.create template with the successful template version ID
// https://github.com/coder/coder/blob/b6703b11c6578b2f91a310d28b6a7e57f0069be6/cli/templatecreate.go#L169-L170

export interface CreateTemplateData {
  name: string
  display_name: string
  description: string
  icon: string
  default_ttl_hours: number
  allow_user_cancel_workspace_jobs: boolean
  parameter_values_by_name?: Record<string, string>
}
interface CreateTemplateContext {
  organizationId: string
  error?: unknown
  jobError?: string
  starterTemplate?: TemplateExample
  exampleId?: string | null // It can be null because it is being passed from query string
  version?: TemplateVersion
  templateData?: CreateTemplateData
  parameters?: ParameterSchema[]
  // file is used in the FE to show the filename and some other visual stuff
  // uploadedFile is the response from the server to use in the API
  file?: File
  uploadResponse?: UploadResponse
}

export const createTemplateMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QGMBOYCGAXMAVMAtgA4A22YAdLFhqlgJYB2UAxANoAMAuoqEQPax6Dfo14gAHogCMAVgDMFDgA5ZATnkKALADY9AdgA0IAJ6I1AJmkVlyi-tk79F2bI4c1AX0-G0mHPjEZDhUNHRMrGzSPEggAkIiYrFSCHKKKuqa8roGxmYI+u4UWgoq0lpqOvJO0t6+6OSBpOQUJPwYEBEAymE4qE3BYCwQopRMAG78ANaUfo2EzSFtHd29YP0LgwgT-MjY9KKcXEfi8cIHSaApOhaK+vrZ8mocOnLK+mp5iMrSyhRqpWUHC0Wh++mUdRAcwCmxay06zB6tD6A3ILHWqH4qAoiwAZliCBRoXhYUt2gioEi6OtUThtoxJntEkcTrEzolxNdbhR7o9nq9ZO9PqZzNILMUXtI1NKAfYtPpIcTaWMICQhgBhABKAFEAIK4bWsviCc6iTmIWQWL4IWT3YoKMWuCzuLQWCE+KENGFBFrQiJEr0RABi9FQ1AAaushKJhqMKDsZgH-CSfSE-cwk-tmCGw1hI2GLvTGftDtwjXETRzkog3X8bm5neplBo3EYRalnBR5PoKs8OBYdGoHLJFV6U4tZoGM+moDmI1GLujUJjsXiCZnvRON-6Z3O8wvREXdiXGCzuKdKxdzalbTp-g7dBxpA9KsL8q6tP99DpZFp5FYOEcCxR2TZVtwzAB3DBzmzLEACl+AAI1wfgACEwHVfggjAHAIFjRgxgZaZJ1A0kSKzKAKCgmDZ3gpCUPQzDsNwo8mQuM8YmNBIr2rAp5GsNQSjleQRMAnRlGtOQOH+GUHAsUEQWkV4QPmVNyIYSDoI02jUAQ5C0IwrDSBwyB8MIyZEyVMjwMo6jtKDOj9MYoy1RYnY2NLY5ogvbizV4h4BKEnsRPkMSJPbexrB7aVlEeQppHkd16lItSbKorTg0chjDOY0yMSxHFgnxVBCSs1KZ3SmiHN0+iDKY4y3KIjzTzLc82UvPyrhkW9734x9nyeQdrRE6xLREywNGbHQOAVD0yq3CqiExYgsAABVoDACBMsMWHVAB5AA5XAAEkDoAVUNNquNNS5JBkKw1AoX4e37Ac3VuSTO3lH9bVcD4LFuJLPRShap0omdlTM+MiMsscwIqiGyNYk8OJ8m7rzkb9evKaaBtfa1lDvWx+wSwmny0GbZuS1TQf8Hcwch-LVyK9d5sGNLEbU5HmVaziK1826Ukxu8AT63GXyG9tfmkkKQop5xu1cbwPUYfgIDgcQ2fINGqy6hAAFo30QfXuRlM3zbUIGtZCahkQiHWeL111rTUBKeSUlRvw0AE9BUzd2fhVZkRpMiHc6u6EGqfQno+Qdm2+qoQQJyoKCqexZGkFQnmBP3x3Z+hVTAMPBZkYKKHsbJpsE1xnyNm07VF6RM8xj4c7muHrJnYvryea0qmKc3ynUTOn1z+GwZsvd82jW72UdiP3kk1QnqfVxAPBH43CtjvyonuzMpqpycoayBu94kFHoUzRf3439LB0T7pNUOQ3EqWOXh0MfO4npajLWjatp9HgO1AWGN+x-A3hoMUdhYryU+uKIcP1hz-UBl-XedNpwM1DiA9GvEEqvC7JoWwLhoH6ClJJHQn5XCWx7C4P8P55BoNpuQCAZ89bPkzjYGBVgLBDlihweQkkZqpxeLyX8Vhmy1GVkAA */
  createMachine(
    {
      id: "createTemplate",
      predictableActionArguments: true,
      schema: {
        context: {} as CreateTemplateContext,
        events: {} as
          | { type: "CREATE"; data: CreateTemplateData }
          | { type: "UPLOAD_FILE"; file: File }
          | { type: "REMOVE_FILE" },
        services: {} as {
          uploadFile: {
            data: UploadResponse
          }
          loadStarterTemplate: {
            data: TemplateExample
          }
          createFirstVersion: {
            data: TemplateVersion
          }
          waitForJobToBeCompleted: {
            data: TemplateVersion
          }
          loadParameterSchema: {
            data: ParameterSchema[]
          }
          createTemplate: {
            data: Template
          }
        },
      },
      tsTypes: {} as import("./createTemplateXService.typegen").Typegen0,
      initial: "starting",
      states: {
        starting: {
          always: [
            { target: "loadingStarterTemplate", cond: "isExampleProvided" },
            { target: "idle" },
          ],
          tags: ["loading"],
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
          tags: ["loading"],
        },
        idle: {
          on: {
            CREATE: {
              target: "creating",
              actions: ["assignTemplateData"],
            },
            UPLOAD_FILE: {
              actions: ["assignFile"],
              target: "uploading",
            },
            REMOVE_FILE: {
              actions: ["removeFile"],
            },
          },
        },
        uploading: {
          invoke: {
            src: "uploadFile",
            onDone: {
              target: "idle",
              actions: ["assignUploadResponse"],
            },
            onError: {
              target: "idle",
              actions: ["displayUploadError", "removeFile"],
            },
          },
        },
        creating: {
          initial: "creatingFirstVersion",
          states: {
            creatingFirstVersion: {
              invoke: {
                src: "createFirstVersion",
                onDone: {
                  target: "waitingForJobToBeCompleted",
                  actions: ["assignVersion"],
                },
                onError: {
                  actions: ["assignError"],
                  target: "#createTemplate.idle",
                },
              },
              tags: ["submitting"],
            },
            waitingForJobToBeCompleted: {
              invoke: {
                src: "waitForJobToBeCompleted",
                onDone: [
                  {
                    target: "loadingMissingParameters",
                    cond: "hasMissingParameters",
                    actions: ["assignVersion"],
                  },
                  {
                    target: "#createTemplate.idle",
                    actions: ["displayJobError"],
                    cond: "hasFailed",
                  },
                  { target: "creatingTemplate", actions: ["assignVersion"] },
                ],
                onError: {
                  target: "#createTemplate.idle",
                  actions: ["assignError"],
                },
              },
              tags: ["submitting"],
            },
            loadingMissingParameters: {
              invoke: {
                src: "loadParameterSchema",
                onDone: {
                  target: "promptParameters",
                  actions: ["assignParameters"],
                },
                onError: {
                  target: "#createTemplate.idle",
                  actions: ["assignError"],
                },
              },
              tags: ["submitting"],
            },
            promptParameters: {
              on: {
                CREATE: {
                  target: "creatingTemplate",
                  actions: ["assignTemplateData"],
                },
              },
            },
            creatingTemplate: {
              invoke: {
                src: "createTemplate",
                onDone: {
                  target: "created",
                  actions: ["onCreate"],
                },
                onError: {
                  actions: ["assignError"],
                  target: "#createTemplate.idle",
                },
              },
              tags: ["submitting"],
            },
            created: {
              type: "final",
            },
          },
        },
      },
    },
    {
      services: {
        uploadFile: (_, { file }) => uploadTemplateFile(file),
        loadStarterTemplate: async ({ organizationId, exampleId }) => {
          if (!exampleId) {
            throw new Error(`Example ID is not defined.`)
          }
          const examples = await getTemplateExamples(organizationId)
          const starterTemplate = examples.find(
            (example) => example.id === exampleId,
          )
          if (!starterTemplate) {
            throw new Error(`Example ${exampleId} not found.`)
          }
          return starterTemplate
        },
        createFirstVersion: async ({ organizationId, exampleId }) => {
          if (!exampleId) {
            throw new Error("No example ID provided")
          }

          return createTemplateVersion(organizationId, {
            storage_method: "file",
            example_id: exampleId,
            provisioner: "terraform",
            tags: {},
          })
        },
        waitForJobToBeCompleted: async ({ version }) => {
          if (!version) {
            throw new Error("Version not defined")
          }

          let status = version.job.status
          while (["pending", "running"].includes(status)) {
            version = await getTemplateVersion(version.id)
            status = version.job.status
          }
          return version
        },
        loadParameterSchema: async ({ version }) => {
          if (!version) {
            throw new Error("Version not defined")
          }

          return getTemplateVersionSchema(version.id)
        },
        createTemplate: async ({
          organizationId,
          version,
          templateData,
          parameters,
        }) => {
          if (!version) {
            throw new Error("Version not defined")
          }

          if (!templateData) {
            throw new Error("Template data not defined")
          }

          const {
            default_ttl_hours,
            parameter_values_by_name,
            ...safeTemplateData
          } = templateData

          // Get parameter values if they are needed/present
          const parameterValues: CreateTemplateRequest["parameter_values"] = []
          if (parameters) {
            parameters.forEach((schema) => {
              const value = parameter_values_by_name?.[schema.name]
              parameterValues.push({
                name: schema.name,
                source_value: value ?? schema.default_source_value,
                destination_scheme: schema.default_destination_scheme,
                source_scheme: schema.default_source_scheme,
              })
            })
          }

          return createTemplate(organizationId, {
            ...safeTemplateData,
            default_ttl_ms: templateData.default_ttl_hours * 60 * 60 * 1000, // Convert hours to ms
            template_version_id: version.id,
            parameter_values: parameterValues,
          })
        },
      },
      actions: {
        assignError: assign({ error: (_, { data }) => data }),
        displayJobError: (_, { data }) => {
          displayError("Provisioner job failed.", data.job.error)
        },
        displayUploadError: () => {
          displayError("Error on upload the file.")
        },
        assignStarterTemplate: assign({
          starterTemplate: (_, { data }) => data,
        }),
        assignVersion: assign({ version: (_, { data }) => data }),
        assignTemplateData: assign({ templateData: (_, { data }) => data }),
        assignParameters: assign({ parameters: (_, { data }) => data }),
        assignFile: assign({ file: (_, { file }) => file }),
        assignUploadResponse: assign({ uploadResponse: (_, { data }) => data }),
        removeFile: assign({
          file: (_) => undefined,
          uploadResponse: (_) => undefined,
        }),
      },
      guards: {
        isExampleProvided: ({ exampleId }) => Boolean(exampleId),
        hasFailed: (_, { data }) => data.job.status === "failed",
        hasMissingParameters: (_, { data }) =>
          Boolean(
            data.job.error && data.job.error.includes("missing parameter"),
          ),
      },
    },
  )
