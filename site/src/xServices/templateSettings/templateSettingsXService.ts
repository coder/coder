import { getTemplateByName, updateTemplateMeta, deleteTemplate } from "api/api"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { createMachine } from "xstate"
import { assign } from "xstate/lib/actions"
import { displaySuccess } from "components/GlobalSnackbar/utils"
import { t } from "i18next"

export const templateSettingsMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QBcwFsAOAbAhqgymMsgJYB2UsAdFgPY4TlQDEEtZYV5AbrQNadUmXASKkK1OgyYIetAMZ4S7ANoAGALqJQGWrBKl22kAA9EANgCMVAKwB2AByWATJbWWAnABYv55w-MAGhAAT0RnAGZnKgibGw9nD3M1JJsHNQiAX0zgoWw8MEJiJmpIAyZmfABBADUAUWNdfUMyYzMESx9bSK8IhwdncwcbF2dgsIRejypzX2dXf09zCLUbbNz0fNFiiSpYHG4Ktg4uMl4BKjyRQrESvYOZOUUW9S0kECbyo3f25KoHOxeDwODx9EYeNTzcYWGxqKgeGw+SJ2cx+VZebI5EBkWgQODGK4FIriSg0eiMCiNPRfVo-RBeMahRAQqhqNnuWERFFeOyedYgQnbEmlRgkqnNZS00DtSwRcz-EY2PzeeYOLk2aEIWJ2WwOIEBNIeBG8-mCm47Un7Q6U96fFptenWRJDIZy+y9AFBJkIEbRbxeWUBRwIyx2U2ba7Eu5WyDimkOhAI2yxSx+foMpWWTV2RLJgIrPoA4bh4RE24SOP2ukdHXOgJq8zuvoozWWBys9nzSydNt2NQYzFAA */
  createMachine(
    {
      id: "templateSettings",
      predictableActionArguments: true,
      tsTypes: {} as import("./templateSettingsXService.typegen").Typegen0,
      schema: {} as {
        context: {
          organizationId: string
          templateName: string
          templateSettings?: Template
          getTemplateError?: unknown
          saveTemplateSettingsError?: unknown
          deleteTemplateError?: Error | unknown
        }
        services: {
          getTemplateSettings: {
            data: Template
          }
          saveTemplateSettings: {
            data: Template
          }
        }
        events:
          | { type: "SAVE"; templateSettings: UpdateTemplateMeta }
          | { type: "DELETE" }
          | { type: "CONFIRM_DELETE" }
          | { type: "CANCEL_DELETE" }
      },
      initial: "loading",
      states: {
        loading: {
          invoke: {
            src: "getTemplateSettings",
            onDone: [
              {
                actions: "assignTemplateSettings",
                target: "editing",
              },
            ],
            onError: {
              target: "error",
              actions: "assignGetTemplateError",
            },
          },
        },
        editing: {
          on: {
            SAVE: {
              target: "saving",
            },
            DELETE: {
              target: "confirmingDelete",
            },
          },
        },
        confirmingDelete: {
          on: {
            CONFIRM_DELETE: {
              target: "deleting",
            },
            CANCEL_DELETE: {
              target: "editing",
            },
          },
        },
        deleting: {
          entry: "clearDeleteTemplateError",
          invoke: {
            src: "deleteTemplate",
            id: "deleteTemplate",
            onDone: [
              {
                target: "deleted",
                actions: "displayDeleteSuccess",
              },
            ],
            onError: [
              {
                actions: "assignDeleteTemplateError",
                target: "editing",
              },
            ],
          },
        },
        deleted: {
          type: "final",
        },
        saving: {
          invoke: {
            src: "saveTemplateSettings",
            onDone: [
              {
                target: "saved",
              },
            ],
            onError: [
              {
                target: "editing",
                actions: ["assignSaveTemplateSettingsError"],
              },
            ],
          },
          tags: ["submitting"],
        },
        saved: {
          entry: "onSave",
          type: "final",
          tags: ["submitting"],
        },
        error: {
          type: "final",
        },
      },
    },
    {
      services: {
        getTemplateSettings: async ({ organizationId, templateName }) => {
          return getTemplateByName(organizationId, templateName)
        },
        saveTemplateSettings: async (
          { templateSettings },
          { templateSettings: newTemplateSettings },
        ) => {
          if (!templateSettings) {
            throw new Error("templateSettings is not loaded yet.")
          }

          return updateTemplateMeta(templateSettings.id, newTemplateSettings)
        },
        deleteTemplate: (ctx) => {
          if (!ctx.templateSettings) {
            throw new Error("Template not loaded")
          }
          return deleteTemplate(ctx.templateSettings.id)
        },
      },
      actions: {
        assignTemplateSettings: assign({
          templateSettings: (_, { data }) => data,
        }),
        assignGetTemplateError: assign({
          getTemplateError: (_, { data }) => data,
        }),
        assignSaveTemplateSettingsError: assign({
          saveTemplateSettingsError: (_, { data }) => data,
        }),
        assignDeleteTemplateError: assign({
          deleteTemplateError: (_, event) => event.data,
        }),
        clearDeleteTemplateError: assign({
          deleteTemplateError: (_) => undefined,
        }),
        displayDeleteSuccess: () =>
          displaySuccess(t("deleteSuccess", { ns: "templatePage" })),
      },
    },
  )
