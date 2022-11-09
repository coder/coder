import { assign, createMachine } from "xstate"
import {
  checkAuthorization,
  getTemplateByName,
  getTemplateDAUs,
  getTemplateVersion,
  getTemplateVersionResources,
  getTemplateVersions,
} from "api/api"
import {
  AuthorizationResponse,
  Template,
  TemplateDAUsResponse,
  TemplateVersion,
  WorkspaceResource,
} from "api/typesGenerated"

export interface TemplateContext {
  organizationId: string
  templateName: string
  template?: Template
  activeTemplateVersion?: TemplateVersion
  templateResources?: WorkspaceResource[]
  templateVersions?: TemplateVersion[]
  templateDAUs?: TemplateDAUsResponse
  permissions?: AuthorizationResponse
  getTemplateError?: Error | unknown
}

const getPermissionsToCheck = (templateId: string) => ({
  canUpdateTemplate: {
    object: {
      resource_type: "template",
      resource_id: templateId,
    },
    action: "update",
  },
})

export const templateMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QAoC2BDAxgCwJYDswBKAOhgBdyCoAVMVABwBt1ywBiCAe0JIIDcuAazAk0WPIVIUq+WvWaswCAV0ytcPANoAGALqJQDLrFxUehkAA9EAJgDsOkgGZbO2wE5bANg8BGABZnHR0AgBoQAE9EPx0-Eg9EpIAOe28-D28ggF9siPEcAmI+fDNcdCYASXwAMy4SLCp+MDpGFjYANTAAJ1MeMjBKagBBTCaWhXawLt7NfE4eUVURMQxCqRKyiuq6hrHcZtbFTp6+-AGhuVHxo6mZs5V8QXVzfF0DJBBjU1fLGwQAgF4t4AKzeWzOTIhEEhZIRaIIZI6EEkEIhZyg2xuEF+XL5NaSYoELZVWr1NhtJQAJTgXAArt1MHALrJ5JS2DTYPTGXAFrxlqICoTSMSqNsySQKccwJzuUzYCzqLdqbSGfLHs8NNp9JZvmULJ9-kDkiRks4QR50SDkh5nH5nPCYjpXKi0SDnObbeaAniQEKiiLSmLSbspXdTnMFTIlZMlPdI3ylk9hIKCQHNsGduTYydZjwo4NWcrc2dYBq1Fq3jrPnrfobEAFQqbQt5kiCcf4go6ECD7Ca0XFMskAmksr7-RtReUQ1xEyRYOQlKsJOmp+K6rqTPr8H9ELaTclkg5wQEzRidB5u-Y0q6QgFbAEsuCMuO0xsmFx0BBIOwACIAUQAGX-Gh-03H45ksBFrycGE7zcW1bD8e0In+Pw3G8EggT8M0-Fbc8PGSV8Vw2TAeBqXBulQahfzAJhBg4ABhAB5AA5AAxSoqQAWQAfQA4DQPA7ddwQZCMVRUF7Bw3svXsEFuz8MFMNtC8bRtHQzWHYj1mKMjako6i5Fo+i2HYRjhlYxigP4oCQLAmstzrUA0OcaSSHsZwbSUjwAl83zwiiGJGw8LCwTtU8vGQnI8j9N9im-UzqDnAUSEShjizAYTnOsRB7BHEglLwoqskCWxFJ8ewPJ0bx8tsC1IQhZwdOFNK6MGZKem6LhuhIY46iotrTImdkssciCDRcvcbVRJrwr8fLXHsRTYRIOCau8WqYRxIjfXwLhv3gT4J2KaM5Ey7LIPrAFyqChBLVvEJPGk2w21PFrVyDacsz2G4c2mCN+jOqBrgOEbpXjSavicq6prEhar1tR7IXSaSfVik7AxJH7GjBzLIfOWA6UweUjqMGGof+TzbEKsEMiRdwYUhbtkhw5HnHvXx0g8D7Jy+9d6lxw5-oJy7Kb3B07vsJDkekjwcSyWxeaJfmZ0lf7ZTVZlgcyzWeTJ6GJp3a7kOWu7YjcR6wQCEFAQfbxlaxzMJTDFUuS1hUiZJuADdrWHcoQVtMJxdtWfvaL0hWm2rfcRXHxBR2M2+l2NdVfWxeNuHbW7Xz+xCWJkm8ZE3LbRO1zV12S0jRVzpFwH8F9inM4D03u2Ux6sWvQj2wTjH4qd5PQzrvMG-nYnSYz0TQRNG3gnUyE+zNNv-EevDrUya9mr7kiVexlPRoJxujdE7O7r8gIO+dXtUkyMvVazSfrt8bt7xpgcFttC1nXsROPy-SBH5w1iO6MKwRghAkbH2BSUtHBrTRPeC8rhxKJ30hRKiNF2psEAS3ZCNMkR4R8MOWqfloEIntCvDmnoRzyUIvLRO6VWTYP+F4TCNt0g22REhV6pCYgEJIPVDENsPBpA9GkehmCAHjREtdVmmFPCKy2u6RwcJzaQicMOb0wQ3ALVxNvXSRAmExBUWQvOA5baqXlrbXIuQgA */
  createMachine(
    {
      id: "templateMachine",
      predictableActionArguments: true,
      tsTypes: {} as import("./templateXService.typegen").Typegen0,
      schema: {
        context: {} as TemplateContext,
        services: {} as {
          getTemplate: {
            data: Template
          }
          getActiveTemplateVersion: {
            data: TemplateVersion
          }
          getTemplateResources: {
            data: WorkspaceResource[]
          }
          getTemplateVersions: {
            data: TemplateVersion[]
          }
          getTemplateDAUs: {
            data: TemplateDAUsResponse
          }
          getTemplatePermissions: {
            data: AuthorizationResponse
          }
        },
      },
      initial: "gettingTemplate",
      states: {
        gettingTemplate: {
          invoke: {
            src: "getTemplate",
            onDone: [
              {
                actions: "assignTemplate",
                target: "initialInfo",
              },
            ],
            onError: [
              {
                actions: "assignGetTemplateError",
                target: "error",
              },
            ],
          },
        },
        initialInfo: {
          type: "parallel",
          states: {
            activeTemplateVersion: {
              initial: "gettingActiveTemplateVersion",
              states: {
                gettingActiveTemplateVersion: {
                  invoke: {
                    src: "getActiveTemplateVersion",
                    onDone: [
                      {
                        actions: "assignActiveTemplateVersion",
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
            templateResources: {
              initial: "gettingTemplateResources",
              states: {
                gettingTemplateResources: {
                  invoke: {
                    src: "getTemplateResources",
                    onDone: [
                      {
                        actions: "assignTemplateResources",
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
            templateVersions: {
              initial: "gettingTemplateVersions",
              states: {
                gettingTemplateVersions: {
                  invoke: {
                    src: "getTemplateVersions",
                    onDone: [
                      {
                        actions: "assignTemplateVersions",
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
            templateDAUs: {
              initial: "gettingTemplateDAUs",
              states: {
                gettingTemplateDAUs: {
                  invoke: {
                    src: "getTemplateDAUs",
                    onDone: [
                      {
                        actions: "assignTemplateDAUs",
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
            templatePermissions: {
              initial: "gettingTemplatePermissions",
              states: {
                gettingTemplatePermissions: {
                  invoke: {
                    src: "getTemplatePermissions",
                    onDone: {
                      target: "success",
                      actions: "assignPermissions",
                    },
                  },
                },
                success: {
                  type: "final",
                },
              },
            },
          },
          onDone: {
            target: "loaded",
          },
        },
        loaded: {
          initial: "waiting",
          states: {
            refreshingTemplate: {
              invoke: {
                id: "refreshTemplate",
                src: "getTemplate",
                onDone: { target: "waiting", actions: "assignTemplate" },
              },
            },
            waiting: {
              after: {
                5000: "refreshingTemplate",
              },
            },
          },
        },
        error: {
          type: "final",
        },
      },
    },
    {
      services: {
        getTemplate: (ctx) =>
          getTemplateByName(ctx.organizationId, ctx.templateName),
        getActiveTemplateVersion: (ctx) => {
          if (!ctx.template) {
            throw new Error("Template not loaded")
          }

          return getTemplateVersion(ctx.template.active_version_id)
        },
        getTemplateResources: (ctx) => {
          if (!ctx.template) {
            throw new Error("Template not loaded")
          }

          return getTemplateVersionResources(ctx.template.active_version_id)
        },
        getTemplateVersions: (ctx) => {
          if (!ctx.template) {
            throw new Error("Template not loaded")
          }

          return getTemplateVersions(ctx.template.id)
        },
        getTemplateDAUs: (ctx) => {
          if (!ctx.template) {
            throw new Error("Template not loaded")
          }
          return getTemplateDAUs(ctx.template.id)
        },
        getTemplatePermissions: (ctx) => {
          if (!ctx.template) {
            throw new Error("Template not loaded")
          }
          return checkAuthorization({
            checks: getPermissionsToCheck(ctx.template.id),
          })
        },
      },
      actions: {
        assignTemplate: assign({
          template: (_, event) => event.data,
        }),
        assignActiveTemplateVersion: assign({
          activeTemplateVersion: (_, event) => event.data,
        }),
        assignGetTemplateError: assign({
          getTemplateError: (_, event) => event.data,
        }),
        assignTemplateResources: assign({
          templateResources: (_, event) => event.data,
        }),
        assignTemplateVersions: assign({
          templateVersions: (_, event) => event.data,
        }),
        assignTemplateDAUs: assign({
          templateDAUs: (_, event) => event.data,
        }),
        assignPermissions: assign({
          permissions: (_, event) => event.data,
        }),
      },
    },
  )
