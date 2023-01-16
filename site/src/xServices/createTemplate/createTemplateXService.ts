import {
  getTemplateExamples,
  createTemplateVersion,
  getTemplateVersion,
  createTemplate,
  getTemplateVersionSchema,
  uploadTemplateFile,
  getTemplateVersionLogs,
} from "api/api"
import {
  CreateTemplateVersionRequest,
  ParameterSchema,
  ProvisionerJobLog,
  Template,
  TemplateExample,
  TemplateVersion,
  UploadResponse,
} from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
import { delay } from "util/delay"
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
  jobLogs?: ProvisionerJobLog[]
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
  /** @xstate-layout N4IgpgJg5mDOIC5QGMBOYCGAXMAVMAtgA4A22YAdLFhqlgJYB2UAxANoAMAuoqEQPax6Dfo14gAHogCMAVgDMFDgA5ZATgBM86dO3z5agDQgAnoi0B2CmosA2WbNsGOF5WpsBfD8bSYc+YjIcKho6JlY2aR4kEAEhETEYqQQ5RRV1LR09A2MzBGVpClkOEo5pABZlZVsNNWllLx90cgDScgoSfgwIcIBlUJxUVqCwFghRSiYAN34Aa0pfFsI24M7uvoGwIeWRhGn+ZGx6UU4uU-E44WPE0GSaxQsLeXLZaTVntQVc8xKlUo59LZbHUXhZGiBFv4du01j1mP1aINhuQWFtUPxUBQVgAzDEECiQvDQ1ZdOFQBF0LbInB7RgzQ4JU7nGKXBLiO5aCiPZ6vd7lT7yb4IWqKF7aN5ucrSYEcWTgwnUyYQEijADCACUAKIAQVwmuZfEEV1E7MQsg0QulHHKFBqxXcNhqWnk8uaUMC7XoytGAFUAAoAGQA8tqACIAfQAYgBJAP67gXI1spLmV5c57yCxyWzWmwOIXyarWDg1R4lDTKDQaaSuvxEj3BL0qlhagCyQYAapqo7H49FDfFrqaEPIOBobQK9C4LLVZELXjanAZ3Pz3LUwd4IW76ytKABXUik8JjCYUfbzAnbxUUA+w8K0+lHE7cA2xJNDlPCtNPcqZ7O5ix81MRBKkUeR1ClcoSlHKVbFrJYG33Q91mYVFUHRTEcTxS862vW8j2YB8DifRgmQTFl3xNT8q3kDQKG0DQs1eJxZHKec7AoAoNAUWVR1qODNwVYkFjdcIcKOZhI3oVBqA7LYhFEE9GEmOk5hE3DhPEhhmC08IpJkrA5Jk64iIZa4yP7N9Byo24QLkIo7Q4NRlABJ55CcS1rVFMdpVAysnI3JoNMQ3SdMhPTpNk+TrjQjCsSCXFUHxISQvCsLRMkyLDOi0RTJIizE2sm5JHMbi6IYpjpXAtjgIQYErGrZQLBsconm45QXUEq9NLSqAKAAdwwK5JIxAApfgACNcH4AAhMBVX4QIwBwCAlJUmYLxS3dQr6wbhqgSMxsm6a5oWpaVryxkX3IgdjWK5JpDHG0oJqNQnPKCtWMtCorA+t4HEqDqXE6oKEO23qBqG7SDqOqbZvmxbSGWyA1rPVTNu61KMt2qG9Nhk6EfOyBLvMl8okKu7hyrc16OkRi5Cqr7auajhbSzDqK0zJiBNB91wexyH9sO1Bxrh07EZVFbUfPdSwZGHbBeh4XRYJs6kYu-YzOfM4NEs1kP1slInooF7anez6aryao6LUWwmuUSpzXNOR4L5+WIb2pX8fhtXJZRtEMXi7BEuSzH+b8MTPbxkXjp9iXkYgEntdffWbJK4Vx3KunKpYy3ECqG06f5fl3P5QKt2C8OJL6u9mFbehYCEZg-VoDACGRmTpfR2W3faCHa6gevG-CFvUDbjvYCT0jrr1yj7pkQCrE0Zz1A66pbe+2xClsLM6hLB1bblLrK-dgWB6HpuoFH8fBlgWLA6wpKtJ3U+I508+G8v6-29vqeCoooqVMtBZ3psxaqnksxKAYixSsjEeYVzln3AWRB0TECwN-CeLANQ6j1CnOew56hOQoJnKCspXAKE0JaCsyg2ZvFok4R6VRXYvyQW-PqvUjIKUYAAdWEAACwwbfLuG0e4sOCBDDhOUeH8MEfJP+M8KbJkNlKWQDluJORcpmQEgpapZFsJxBwFZdD2HKLYD6zDrwSOxpw64vCsACNbj-eS99MIJWwltV+1cdo2NEHYhxY8nEyXkWcG6VlKafhUWo+0mi3IeV0b+RQbgHYWBcNbAo5cPGsK8b1RUwi1LP0sQLHJwlgl4MAZ+aQi9rC1FUM5QswJbBCiag1eozUpQzgsB9DQFiepFOxrkgOrjg7uLDp46GO1FSlNCaneeGdaK01AYzPOCAbBWFkK4H6spijViPpuRg-AIBwHEJknAiiDbpwALRGFqhc1RB97n3JBgg3uwRqCInCGctOyQPpNP0dKIEKTCw6H5DvHpIUB4UiRMJT5sz3JWEqbbR4NQgStSeEKasdEZz2CcBWFw7gmpgu2k2MAMKqauFZs8VwTUVAFDUC8IUW99EqCzO5YoZQS6EvlvhFCUBSURItLVKCVgSiuFgjvT4spOVZOhnyw2ORmZPGsC8KC7k3gzlkA0Y+iDxF9LYfpKKxk04zIIesig-1RxuBsBoMoQJPKMVtBWO2GrrWfHKOUKVOq2GK2jirOORMICyvTik4V7xOkHzFAKvIOhrXEMqKYt6FQmqsQ9T3MSH9h7N0cRPQND1Kj6JBPUYEtFKzOW+i8TiK8NBOCxSylNCsUGI3QVm2+OafglmsBUKtegTHKEtAxM1jtzSZmXgSrVLzU3pTYT46R9jZEyVbfkCwlooJqC5GyqojhxzZzrVYthioF0MxtHYDqjxXgA10L8wobqFAMwBtUTVvMxETvYduANADwmG2te2kEXbjGsV7bVNwS8xTPCMTYMcXgvBAA */
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
          loadVersionLogs: {
            data: ProvisionerJobLog[]
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
              cond: "isNotUsingExample",
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
                    target: "loadingVersionLogs",
                    actions: ["assignJobError", "assignVersion"],
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
            loadingVersionLogs: {
              invoke: {
                src: "loadVersionLogs",
                onDone: {
                  target: "#createTemplate.idle",
                  actions: ["assignJobLogs"],
                },
                onError: {
                  target: "#createTemplate.idle",
                  actions: ["assignError"],
                },
              },
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
                source_scheme: "data",
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
            // Delay the verification in two seconds to not overload the server
            // with too many requests Maybe at some point we could have a
            // websocket for template version Also, preferred doing this way to
            // avoid a new state since we don't need to reflect it on the UI
            await delay(2_000)
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
        loadVersionLogs: ({ version }) => {
          if (!version) {
            throw new Error("Version is not set")
          }

          return getTemplateVersionLogs(version.id)
        },
      },
      actions: {
        assignError: assign({ error: (_, { data }) => data }),
        assignJobError: assign({ jobError: (_, { data }) => data.job.error }),
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
        assignJobLogs: assign({ jobLogs: (_, { data }) => data }),
      },
      guards: {
        isExampleProvided: ({ exampleId }) => Boolean(exampleId),
        isNotUsingExample: ({ exampleId }) => !exampleId,
        hasFile: ({ file }) => Boolean(file),
        hasFailed: (_, { data }) => data.job.status === "failed",
        hasMissingParameters: (_, { data }) =>
          Boolean(
            data.job.error && data.job.error.includes("missing parameter"),
          ),
      },
    },
  )
