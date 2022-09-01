import { assign, createMachine } from "xstate"
import {
  getTemplateByName,
  getTemplateDAUs,
  getTemplateVersion,
  getTemplateVersionResources,
  getTemplateVersions,
} from "../../api/api"
import {
  Template,
  TemplateDAUsResponse,
  TemplateVersion,
  WorkspaceResource,
} from "../../api/typesGenerated"

interface TemplateContext {
  organizationId: string
  templateName: string
  template?: Template
  activeTemplateVersion?: TemplateVersion
  templateResources?: WorkspaceResource[]
  templateVersions?: TemplateVersion[]
  templateDAUs: TemplateDAUsResponse
}

export const templateMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QAoC2BDAxgCwJYDswBKAOhgBdyCoAVMVABwBt1ywBiCAe0JIIDcuAazAk0WPIVIUq+WvWaswCAV0ytcPANoAGALqJQDLrFxUehkAA9EAJgDsOkgGYd9gIwBOTwBYdnhzd7ABoQAE9Edx13Eh9PHTcANgAOAFYEnR9U5wBfHNDxHAJiPnwzXHQmAEl8ADMuEiwqfjA6RhY2ADUwACdTHjIwSmoAQUxm1oUOsG6+zXxOHlFVETEMIqlS8sqa+sbx3Ba2xS7e-vxB4bkxiePp2fOVfEF1c3xdAyQQY1M3yxsED4fDFEsDHO5Uql3IlnM4sqEIghkpkXJ4Usl4rZkskPPY8gV1pISgRttU6g02O0lAAlOBcACuPUwcEusnkVLYtNgDKZcEWvBWokKRNIJKoO3JJEpJzAXJ5zNgrOodxpdMZCqeLw02n0lh+5QsXwBwOSJGStixzlsMM87mR1oRkR0zhizlSiVscRdVrctnxIGFxVFZXFZL20vuZ3mipkyqmSge0f5y2ewiFhKDW1Duwp8dOcx4MaGbJV+fOsE1am1711X31fyNiD8PhI9gCtq8cNSjmSjoQ3fssWc9lSWPsQOyiX9gc2YoqYdzHLAABERgBVItXdky1cb5OlQSrGfEkPznNSvMr9ebkuX3cV1SveYfPUmA34f6IZzJZwkLEYsdfGyYc+3cWxUlNF1u1hRJIU8ccp3yAMM1nU8JXqfdYHIJQ1gkTM53QrhX1+eZPwQTxvzNCF4OSRJ3Hsa03D7bFPBcZEGMSNsEmSHxEKQ-AuAgOBLGPaRizjJdiPfMifFsZjWM8bE4SBRTskcXIkNErMz0lJpDkmJdEwGWNrgOI5LyMj86zfBtQABdx6L7NtbDNTwoXdTjx0yP1NJQk9SXPPTzMMqMBlgelMAVeBrJIw07MQewrRIKJvxNHxcXsEJwkQZJ6LNNJbDhC00jczxpz84MAt0syDJlSypNs6xEAopzbCceJHFgnROLa81yrw1CqvDS85XVFkTO3aZRt5aKjBs0jGwQByssRKIvDNVIfGcdtnVcH9+o2fzs0lCNVW5MbFXCyK4Fm755ripqEBSRJYlgtJUniTadB40DMhbHj0ratyPB0HyCQGo6dOGpdpoVBqFvi8jnD7XxB1omFolsCiHMKg6RW0wiLxCgt8BvCS6tC0n4Ye+zHOy-tYJICj3QY2FeOHPH8LQhciYpknLoiqLqasxG3tiHQPrc4FQfsRJPD7D72sYhJvx4vbOcG47ob58thbIlr6a9Jn0og30gWBdwfA1yHCdOth7yVORSyvDc9cW5bQM9F7cu-TKvbcTbrcqrXFx3a8SCuoWYukxa6NSVFzTRWEsfsc1PY8M1MjcbFITlnEg4Jnm7Zd276wRx6DdW+j-vA1P3TdHE0QLgiFzd0X5cN2wYlKnwMQc2Sslx3yIdIJguHQISIDbx6omyFxElg0EAghZxEmYn8zVTtqGPg+iRyD6f7N7em7TyPIgA */
  createMachine(
    {
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
        },
      },
      id: "(machine)",
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
          },
          onDone: {
            target: "loaded",
          },
        },
        loaded: {},
      },
    },
    {
      services: {
        getTemplate: (ctx) => getTemplateByName(ctx.organizationId, ctx.templateName),
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
      },
      actions: {
        assignTemplate: assign({
          template: (_, event) => event.data,
        }),
        assignActiveTemplateVersion: assign({
          activeTemplateVersion: (_, event) => event.data,
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
      },
    },
  )
