import {
  getTemplateExamples,
  createTemplateVersion,
  getTemplateVersion,
  createTemplate,
  getTemplateVersionSchema,
  uploadTemplateFile,
} from "api/api"
import {
  CreateTemplateVersionRequest,
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
  /** @xstate-layout N4IgpgJg5mDOIC5QGMBOYCGAXMAVMAtgA4A22YAdLFhqlgJYB2UAxANoAMAuoqEQPax6Dfo14gAHogCMAVgDMFDgA5ZATlkAWaR1kA2TcukAaEAE9E8jgCYKa62r0B2Wao7zlatQF9vptJg4+MRkOFQ0dEysbNI8SCACQiJi8VIIcooq6lo6+oYm5ohGFJpeXk7W1h5O8nK+-ujkwaTkFCT8GBBRAMoROKjNoWAsEKKUTABu-ADWlAFNhC1h7Z09fWADi0MIk-zI2PSinFzH4onChymgaXpVFE41mq6aHJpO5aYW6V52ZWo10mssmkymUTnqIHmQS2rRWXWYvVo-UG5BYG1Q-FQFCWADNMQQKFC8DDlh14VBEXQNiicDtGFN9sljqd4udkuIbncHvInsoXm8PoUEA5FE9arprAZHHplBCiTTxhASMMAMIAJQAogBBXAall8QQXUQcxCyayfGRmtQlDg6TR6eRS+SVOWNaEhVr0JXDACqAAUADIAeS1ABEAPoAMQAkgG9dwzob2alENZtPcefI1PInG8OI5zUKPHo7Bxbgp3qCVKpXYFiR6wl7lSxNQBZIMANQ1Udj8biBqSlxNCHkOc0SmzaiMwOkTnzFvSmh5FCs7w4lb0OgetYWDcoAFdSGSoiMxhRdrNCW760sD0fVsw6QyDkduPqEkmhynhenHlmx3OBYLoYijyOoHjSGoKgvPmO7ureFCHnCJ7opi2KhHiqAEvKJJ3shj67IylzMgmrKfsa35AtY0glAoVSAtIjGzgushOCWRhpp4yh6NBoJwTeQxXoEURCQczCRvQqDUB2GxCKIp6MOM9IzHM14KqJDDMBpUQSVJWAyVJlxPnsL6MCR-YfoOFHXIg7yKA61jKB4CjSKOzoLnaJbUU81gqOuHg5vx6lQiJIXiZJ0myZcaKoBiWK4viGkCa0YVQNp4V6QZcmMMZRGvicpEDkaVySKmZo0WK9HUUxTgeTytilKUjG3I5VFBbh6VpQA7hgFziZiABS-AAEa4PwABCYAqvwIRgDgEAKUpUyXjhe6dRQPV9VAkaDSNY2TdNs3zblpnmYmVklWk0gvA1bGuNYgGuTmHm6CWThGCCDjQfmsp+JCakdalG29Zp227aNE1TTNpBzZAi3nspK0A2tQObaDO2oENEMHdDyrHYRp1vrE53FcOQK1LRzq1NVjG1UKa4UM4IIKDyLxyLI7Uo26Ilozp4P7VDR1w6MikI8tql1sF3Nabz-WY3tkOHTD+PKXlZlvtYFlsl+NmLjYJR3coD35k9dNfI4oGOeupSyOoLWcwhqMg3z8vY4Lytw6h8UYYlq2O9L3XO3LWMC0reOQCdTJvoVlmk5R5WU1VTHMUKoLjoCpSbg4BgOpoDuCUD+FQK29CwEIzB+rQGAELDUnwxeEu7v7wlaUXJdl1EleoNXtewJHxHR1r5GXTIm4lhwE8SvYVggvIdWuIzuaaNRViaE14J-X7BcB20x7MO35dQF3Pf9LAMVxeh2CYdhyPN2JaVt6Xh-HzXp-9-l77a9ZpXCgnlXU8nM2MglyyDsIYW0GgXAPXkPnFKO8iAYmIFgF+vcWDqm1LqT+w8yZGxXFOJcLxQR6FkNBBc2YnBKEYoQkC2gniwLCEDVKWVLgAHVhAAAsUGn3rojRu8Ft4tzSkwqKog2FYE4VXV+sl37qwKkPC6w47KM2dE5VQtQ3KFnNmae4m5tCOS0NmVw9C+GhQDsw0RHCuGyXPmhBKWEkpS0EZ1cxjAxESO7lIqSMizpkQUfHdM-8GI1QXHof4FBqJgjFGoOQzgN4NEloDHeqUFQ8PFg4xJTjkm4W8YPEmyZdYr3HhPaQPFHKGD5EAhATlQFgTkHIB6kowQc03rfAR99OopK9pfLA190lc0yQHBUOS5F5J1j-KiFU6IAOCUKf4FDWLKDLPYQw0TXK+D+owfgEA4DiC3uQUZ380gAFo1ALiOaAv4ly-gGGafEpuglqBIiiAckeCBl4LgcOONiVhbTODBCU66xjd4PgpOsTYe4XnDkdBQ2c-x8wTx5PmN4HyHiL0cC4Kw913hAqbGASFlEvDjloQ9KC3FPD2g8goUBbEdCOnzPoMoQKkJ7ygPi3W7yhQChXGBbMD05Dri8DAlpCT+n3zZT-LMC5cwUNUBkVw7hZxgSBYwgOulIqGW-l-V5tsODhOIQYJF3FIKaJkLPcJmY1CGGBHyewyqd6yzBq7UOuNYYQHFVdLIFASnaEYloMsQIXoOEZuuRZVqpxODqMK+5cCnGPw7hXSRvd3UyG4uOfBS5JTaA8K8Oq6YqhLgqDYR09ohV3P4TG9pCDobIMTafZNVTKmuRcEoWJVLNysVtnagZgiXFuKsVJet4ClBzlqFBEhHg3iaDIY6OwYIpyOHArcOJ-0RV31Bh03C9aQT2AoKo5wy9OIRrIZuXdrwc66Dck5Lt7SiRut8XHApiL7gKoeLcRZjEFyqHHu9fMAUYK1HWd4IAA */
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
          createVersionWithParameters: {
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
              cond: "isFromScratch",
            },
            REMOVE_FILE: {
              actions: ["removeFile"],
              cond: "hasFile",
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
                  target: "creatingVersionWithParameters",
                  actions: ["assignTemplateData"],
                },
              },
            },
            creatingVersionWithParameters: {
              invoke: {
                src: "createVersionWithParameters",
                onDone: {
                  target: "waitingForJobToBeCompleted",
                  actions: ["assignVersion"],
                },
                onError: {
                  actions: ["assignError"],
                  target: "promptParameters",
                },
              },
              tags: ["submitting"],
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
        createFirstVersion: async ({
          organizationId,
          exampleId,
          uploadResponse,
        }) => {
          if (exampleId) {
            return createTemplateVersion(organizationId, {
              storage_method: "file",
              example_id: exampleId,
              provisioner: "terraform",
              tags: {},
            })
          }

          if (uploadResponse) {
            return createTemplateVersion(organizationId, {
              storage_method: "file",
              file_id: uploadResponse.hash,
              provisioner: "terraform",
              tags: {},
            })
          }

          throw new Error("No file or example provided")
        },
        createVersionWithParameters: async ({
          organizationId,
          parameters,
          templateData,
          version,
        }) => {
          if (!version) {
            throw new Error("No previous version found")
          }
          if (!templateData) {
            throw new Error("No template data defined")
          }

          const { parameter_values_by_name } = templateData
          // Get parameter values if they are needed/present
          const parameterValues: CreateTemplateVersionRequest["parameter_values"] =
            []
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

          return createTemplateVersion(organizationId, {
            storage_method: "file",
            file_id: version.job.file_id,
            provisioner: "terraform",
            parameter_values: parameterValues,
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
        createTemplate: async ({ organizationId, version, templateData }) => {
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

          return createTemplate(organizationId, {
            ...safeTemplateData,
            default_ttl_ms: templateData.default_ttl_hours * 60 * 60 * 1000, // Convert hours to ms
            template_version_id: version.id,
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
        isExampleProvided: ({ exampleId }) => exampleId !== undefined,
        isFromScratch: ({ exampleId }) => exampleId === undefined,
        hasFile: ({ file }) => file !== undefined,
        hasFailed: (_, { data }) => data.job.status === "failed",
        hasMissingParameters: (_, { data }) =>
          Boolean(
            data.job.error && data.job.error.includes("missing parameter"),
          ),
      },
    },
  )
