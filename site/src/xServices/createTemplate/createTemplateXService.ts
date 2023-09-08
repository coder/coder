import {
  getTemplateExamples,
  createTemplateVersion,
  getTemplateVersion,
  createTemplate,
  uploadTemplateFile,
  getTemplateVersionLogs,
  getTemplateVersionVariables,
  getTemplateByName,
} from "api/api";
import {
  ProvisionerJob,
  ProvisionerJobLog,
  ProvisionerType,
  Template,
  TemplateExample,
  TemplateVersion,
  TemplateVersionVariable,
  UploadResponse,
  VariableValue,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import {
  TemplateAutostopRequirementDaysValue,
  calculateAutostopRequirementDaysValue,
} from "pages/TemplateSettingsPage/TemplateSchedulePage/TemplateScheduleForm/AutostopRequirementHelperText";
import { delay } from "utils/delay";
import { assign, createMachine } from "xstate";

// for creating a new template:
// 1. upload template tar or use the example ID
// 2. create template version
// 3. wait for it to complete
// 4. verify if template has missing parameters or variables
//    a. prompt for params
//    b. create template version again with the same file hash
//    c. wait for it to complete
// 5.create template with the successful template version ID
// https://github.com/coder/coder/blob/b6703b11c6578b2f91a310d28b6a7e57f0069be6/cli/templatecreate.go#L169-L170

const provisioner: ProvisionerType =
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Playwright needs to use a different provisioner type!
  typeof (window as any).playwright !== "undefined" ? "echo" : "terraform";

export interface CreateTemplateData {
  name: string;
  display_name: string;
  description: string;
  icon: string;
  default_ttl_hours: number;
  max_ttl_hours: number;
  autostop_requirement_days_of_week: TemplateAutostopRequirementDaysValue;
  autostop_requirement_weeks: number;
  allow_user_autostart: boolean;
  allow_user_autostop: boolean;
  allow_user_cancel_workspace_jobs: boolean;
  parameter_values_by_name?: Record<string, string>;
  user_variable_values?: VariableValue[];
  allow_everyone_group_access: boolean;
}
interface CreateTemplateContext {
  organizationId: string;
  error?: unknown;
  jobError?: string;
  jobLogs?: ProvisionerJobLog[];
  starterTemplate?: TemplateExample;
  exampleId?: string | null; // It can be null because it is being passed from query string
  version?: TemplateVersion;
  templateData?: CreateTemplateData;
  variables?: TemplateVersionVariable[];
  // file is used in the FE to show the filename and some other visual stuff
  // uploadedFile is the response from the server to use in the API
  file?: File;
  uploadResponse?: UploadResponse;
  // When wanting to duplicate a Template
  templateNameToCopy: string | null; // It can be null because it is passed from query string
  copiedTemplate?: Template;
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
            data: UploadResponse;
          };
          loadStarterTemplate: {
            data: TemplateExample;
          };
          createFirstVersion: {
            data: TemplateVersion;
          };
          createVersionWithParametersAndVariables: {
            data: TemplateVersion;
          };
          waitForJobToBeCompleted: {
            data: TemplateVersion;
          };
          checkParametersAndVariables: {
            data: {
              variables?: TemplateVersionVariable[];
            };
          };
          createTemplate: {
            data: Template;
          };
          loadVersionLogs: {
            data: ProvisionerJobLog[];
          };
          copyTemplateData: {
            data: {
              template: Template;
              version: TemplateVersion;
              variables: TemplateVersionVariable[];
            };
          };
        },
      },
      tsTypes: {} as import("./createTemplateXService.typegen").Typegen0,
      initial: "starting",
      states: {
        starting: {
          always: [
            { target: "loadingStarterTemplate", cond: "isExampleProvided" },
            {
              target: "copyingTemplateData",
              cond: "isTemplateIdToCopyProvided",
            },
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
        copyingTemplateData: {
          invoke: {
            src: "copyTemplateData",
            onDone: [
              {
                target: "creating.promptParametersAndVariables",
                actions: ["assignCopiedTemplateData"],
                cond: "hasParametersOrVariables",
              },
              {
                target: "idle",
                actions: ["assignCopiedTemplateData"],
              },
            ],
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
                    target: "loadingVersionLogs",
                    actions: ["assignJobError", "assignVersion"],
                    cond: "hasFailed",
                  },
                  {
                    target: "checkingParametersAndVariables",
                    actions: ["assignVersion"],
                  },
                ],
                onError: {
                  target: "#createTemplate.idle",
                  actions: ["assignError"],
                },
              },
              tags: ["submitting"],
            },
            checkingParametersAndVariables: {
              invoke: {
                src: "checkParametersAndVariables",
                onDone: [
                  {
                    target: "creatingTemplate",
                    cond: "hasNoParametersOrVariables",
                  },
                  {
                    target: "promptParametersAndVariables",
                    actions: ["assignParametersAndVariables"],
                  },
                ],
                onError: {
                  target: "#createTemplate.idle",
                  actions: ["assignError"],
                },
              },
              tags: ["submitting"],
            },
            promptParametersAndVariables: {
              on: {
                CREATE: {
                  target: "creatingVersionWithParametersAndVariables",
                  actions: ["assignTemplateData"],
                },
              },
            },
            creatingVersionWithParametersAndVariables: {
              invoke: {
                src: "createVersionWithParametersAndVariables",
                onDone: {
                  target: "waitingForJobToBeCompleted",
                  actions: ["assignVersion"],
                },
                onError: {
                  actions: ["assignError"],
                  target: "promptParametersAndVariables",
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
          },
        },
      },
    },
    {
      services: {
        uploadFile: (_, { file }) => uploadTemplateFile(file),
        loadStarterTemplate: async ({ organizationId, exampleId }) => {
          if (!exampleId) {
            throw new Error(`Example ID is not defined.`);
          }
          const examples = await getTemplateExamples(organizationId);
          const starterTemplate = examples.find(
            (example) => example.id === exampleId,
          );
          if (!starterTemplate) {
            throw new Error(`Example ${exampleId} not found.`);
          }
          return starterTemplate;
        },
        copyTemplateData: async ({ organizationId, templateNameToCopy }) => {
          if (!organizationId) {
            throw new Error("No organization ID provided");
          }
          if (!templateNameToCopy) {
            throw new Error("No template name to copy provided");
          }
          const template = await getTemplateByName(
            organizationId,
            templateNameToCopy,
          );
          const [version, variables] = await Promise.all([
            getTemplateVersion(template.active_version_id),
            getTemplateVersionVariables(template.active_version_id),
          ]);

          return {
            template,
            version,
            variables,
          };
        },
        createFirstVersion: async ({
          organizationId,
          templateNameToCopy,
          exampleId,
          uploadResponse,
          version,
        }) => {
          if (exampleId) {
            return createTemplateVersion(organizationId, {
              storage_method: "file",
              example_id: exampleId,
              provisioner: provisioner,
              tags: {},
            });
          }

          if (templateNameToCopy) {
            if (!version) {
              throw new Error(
                "Can't copy template due to a missing template version",
              );
            }

            return createTemplateVersion(organizationId, {
              storage_method: "file",
              file_id: version.job.file_id,
              provisioner: provisioner,
              tags: {},
            });
          }

          if (uploadResponse) {
            return createTemplateVersion(organizationId, {
              storage_method: "file",
              file_id: uploadResponse.hash,
              provisioner: provisioner,
              tags: {},
            });
          }

          throw new Error("No file or example provided");
        },
        createVersionWithParametersAndVariables: async ({
          organizationId,
          templateData,
          version,
        }) => {
          if (!version) {
            throw new Error("No previous version found");
          }
          if (!templateData) {
            throw new Error("No template data defined");
          }

          return createTemplateVersion(organizationId, {
            storage_method: "file",
            file_id: version.job.file_id,
            provisioner: provisioner,
            user_variable_values: templateData.user_variable_values,
            tags: {},
          });
        },
        waitForJobToBeCompleted: async ({ version }) => {
          if (!version) {
            throw new Error("Version not defined");
          }

          let job = version.job;
          while (isPendingOrRunning(job)) {
            version = await getTemplateVersion(version.id);
            job = version.job;

            // Delay the verification in two seconds to not overload the server
            // with too many requests Maybe at some point we could have a
            // websocket for template version Also, preferred doing this way to
            // avoid a new state since we don't need to reflect it on the UI
            if (isPendingOrRunning(job)) {
              await delay(2_000);
            }
          }
          return version;
        },
        checkParametersAndVariables: async ({ version }) => {
          if (!version) {
            throw new Error("Version not defined");
          }

          let promiseVariables: Promise<TemplateVersionVariable[]> | undefined =
            undefined;

          if (isMissingVariables(version)) {
            promiseVariables = getTemplateVersionVariables(version.id);
          }

          const [variables] = await Promise.all([promiseVariables]);

          return {
            variables,
          };
        },
        createTemplate: async ({ organizationId, version, templateData }) => {
          if (!version) {
            throw new Error("Version not defined");
          }

          if (!templateData) {
            throw new Error("Template data not defined");
          }

          const {
            default_ttl_hours,
            max_ttl_hours,
            parameter_values_by_name,
            allow_everyone_group_access,
            autostop_requirement_days_of_week,
            autostop_requirement_weeks,
            ...safeTemplateData
          } = templateData;

          return createTemplate(organizationId, {
            ...safeTemplateData,
            disable_everyone_group_access:
              !templateData.allow_everyone_group_access,
            default_ttl_ms: templateData.default_ttl_hours * 60 * 60 * 1000, // Convert hours to ms
            max_ttl_ms: templateData.max_ttl_hours * 60 * 60 * 1000, // Convert hours to ms
            template_version_id: version.id,
            autostop_requirement: {
              days_of_week: calculateAutostopRequirementDaysValue(
                templateData.autostop_requirement_days_of_week,
              ),
              weeks: templateData.autostop_requirement_weeks,
            },
          });
        },
        loadVersionLogs: ({ version }) => {
          if (!version) {
            throw new Error("Version is not set");
          }

          return getTemplateVersionLogs(version.id);
        },
      },
      actions: {
        assignError: assign({ error: (_, { data }) => data }),
        assignJobError: assign({ jobError: (_, { data }) => data.job.error }),
        displayUploadError: () => {
          displayError("Error on upload the file.");
        },
        assignStarterTemplate: assign({
          starterTemplate: (_, { data }) => data,
        }),
        assignVersion: assign({ version: (_, { data }) => data }),
        assignTemplateData: assign({ templateData: (_, { data }) => data }),
        assignParametersAndVariables: assign({
          variables: (_, { data }) => data.variables,
        }),
        assignFile: assign({ file: (_, { file }) => file }),
        assignUploadResponse: assign({ uploadResponse: (_, { data }) => data }),
        removeFile: assign({
          file: (_) => undefined,
          uploadResponse: (_) => undefined,
        }),
        assignJobLogs: assign({ jobLogs: (_, { data }) => data }),
        assignCopiedTemplateData: assign({
          copiedTemplate: (_, { data }) => data.template,
          version: (_, { data }) => data.version,
          variables: (_, { data }) => data.variables,
        }),
      },
      guards: {
        isExampleProvided: ({ exampleId }) => Boolean(exampleId),
        isTemplateIdToCopyProvided: ({ templateNameToCopy }) =>
          Boolean(templateNameToCopy),
        isNotUsingExample: ({ exampleId }) => !exampleId,
        hasFile: ({ file }) => Boolean(file),
        hasFailed: (_, { data }) =>
          Boolean(data.job.status === "failed" && !isMissingVariables(data)),
        hasNoParametersOrVariables: (_, { data }) =>
          data.variables === undefined,
        hasParametersOrVariables: (_, { data }) => {
          return data.variables.length > 0;
        },
      },
    },
  );

const isMissingVariables = (version: TemplateVersion) => {
  return Boolean(
    version.job.error_code &&
      version.job.error_code === "REQUIRED_TEMPLATE_VARIABLES",
  );
};

const isPendingOrRunning = (job: ProvisionerJob) => {
  return job.status === "pending" || job.status === "running";
};
