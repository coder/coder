import {
  checkAuthorization,
  createWorkspace,
  getTemplateByName,
  getTemplateVersionRichParameters,
} from "api/api";
import {
  CreateWorkspaceRequest,
  Template,
  TemplateVersionParameter,
  User,
  Workspace,
  WorkspaceBuildParameter,
} from "api/typesGenerated";
import { assign, createMachine } from "xstate";
import { paramsUsedToCreateWorkspace } from "utils/workspace";

export type CreateWorkspaceMode = "form" | "auto";

type CreateWorkspaceContext = {
  organizationId: string;
  templateName: string;
  mode: CreateWorkspaceMode;
  defaultName: string;
  // Not exposed in the form yet, but can be set as a search param to
  // create a workspace with a specific version of a template
  versionId?: string;
  error?: unknown;
  // Form
  template?: Template;
  parameters?: TemplateVersionParameter[];
  permissions?: Record<string, boolean>;
  // Used on auto-create
  defaultBuildParameters?: WorkspaceBuildParameter[];
};

type CreateWorkspaceEvent = {
  type: "CREATE_WORKSPACE";
  request: CreateWorkspaceRequest;
  owner: User;
};

type RefreshGitAuthEvent = {
  type: "REFRESH_GITAUTH";
};

export const createWorkspaceMachine =
  /** @xstate-layout N4IgpgJg5mDOIC5QGMBOYCGAXMB1A9qgNawAOGyYAyltmAHTIAWYyRAlgHZQCy+EYAMQBtAAwBdRKFL5Y7LO3ycpIAB6IATABYAnPR2iDWgOwBmAGynRo4wFZbAGhABPRDoAc9d+8OnTARlF-Uy0NUQ0AXwinNEwcAmIyCmpaHEYWNi5efiFhf0kkEBk5BSUVdQQNU1svEPcq9wsDU3ctJ1dKv3pRd3NzHVNjf1sjHX8omPQ6BJJySho6egwAVyx8AGEphW5BCCUGLgA3fCIGWOnCWeSFtJW1zbishCP8ZGxFTjFxL5Vi+Q-yoh7MZ9MZjLoQqItN5jDp2ogALT+PSWWwaDR9dzGDTeXTuCYgc7xS5JeapBh3DZbLKCMCoVCEeikAA22AAZoQALaMLZ4ElzFKLSkPd7cZ6cY5vUqfCQ-Qp-aWAhAtPQtWxDaE+OxQ4zwhD+dw1US2cw4-zmLQ9WyicwEol8xICm4MZn4DAQLIAMS5ABFsBhdvt6C9Tjy4g6rmTFq73V7ff7xZL3kovnLpLJ-mVChVzNb6P5tDoMei7P5-G0XIgQqZ6BjbFrWuZGrC7byZqTBWkYx7uN7UJy-bRafTGSz2VywxdHddyfRu3H+4OMInXsmZd8JL8M4rs4hejWCzYLOYDSfjOY9SENPpgmYy+bgjodLbooS2-yZ4t2BBmUJ1gAlABRABBAAVQCAH1cAAeX-ABpKgAAVgPWQC0yKbcAV3BBLHMWs61EQZjQ0ZF3D1ewa36S0QlhasQlbcN2ydWciSyJjkkDTgDglE4znfacozSVjuHYygVylD5U03eVMKzUAc1MPRGh0cEtBMJ9Wj1MxPFsKwmg0DwwmGBip0jTs+MeESP0oYcGVQJlWSwDl+0nYkBPM1y2OssBxLXKSCnTEosPkxBQn8eh7FaewBjRU1THIwIb2CLQ+nrXN1SiV9OByeBCntUTzK3IK5LUKstFrWE4uNGxwQxPUkRsfNqnRfxzybE1+hMtyzOddJWA4bg+AEIrM2UbD6gq58qmqsFQgvSsEGfehLEMExWp0LRSK6iMO164VqW4EadxC5Ui2W0wNDBDVekfLTrx8cIAnBKxgVsbaCt6+de3jWgjuC0qcIsZbfBtPE-F1BaGn0AzIWxGK-HxV98u83rv1-P6SpzSx6EUtT+kIsYT38PUtFscKDTUw1zGxMmy3elGWIOqACoxsaTtNPCLs2-oGlNVq9SJ2tQcCS0eZS+n3N6+0IFZpV3A2-MtBadx1uRcIyIWuxPDsforEM6x7AlnrZ27QCR1QWXxthHHLrBJ8nxtJsSd0ehsQsJWNHrYYi0yiIgA */
  createMachine(
    {
      id: "createWorkspaceState",
      predictableActionArguments: true,
      tsTypes: {} as import("./createWorkspaceXService.typegen").Typegen0,
      schema: {
        context: {} as CreateWorkspaceContext,
        events: {} as CreateWorkspaceEvent | RefreshGitAuthEvent,
        services: {} as {
          loadFormData: {
            data: {
              template: Template;
              permissions: CreateWSPermissions;
              parameters: TemplateVersionParameter[];
            };
          };
          createWorkspace: {
            data: Workspace;
          };
          autoCreateWorkspace: {
            data: Workspace;
          };
        },
      },
      initial: "checkingMode",
      states: {
        checkingMode: {
          always: [
            {
              target: "autoCreating",
              cond: ({ mode }) => mode === "auto",
            },
            { target: "loadingFormData" },
          ],
        },
        autoCreating: {
          invoke: {
            src: "autoCreateWorkspace",
            onDone: {
              actions: ["onCreateWorkspace"],
            },
            onError: {
              actions: ["assignError"],
              target: "loadingFormData",
            },
          },
        },
        loadingFormData: {
          invoke: {
            src: "loadFormData",
            onDone: {
              target: "idle",
              actions: ["assignFormData"],
            },
            onError: {
              target: "loadError",
              actions: ["assignError"],
            },
          },
        },
        idle: {
          on: {
            CREATE_WORKSPACE: {
              target: "creatingWorkspace",
            },
          },
        },
        creatingWorkspace: {
          entry: "clearError",
          invoke: {
            src: "createWorkspace",
            onDone: {
              actions: ["onCreateWorkspace"],
              target: "created",
            },
            onError: {
              actions: ["assignError"],
              target: "idle",
            },
          },
        },
        created: {
          type: "final",
        },
        loadError: {
          type: "final",
        },
      },
    },
    {
      services: {
        createWorkspace: (
          { organizationId, versionId },
          { request, owner },
        ) => {
          if (versionId) {
            request = {
              ...request,
              template_id: undefined,
              template_version_id: versionId,
            };
          }

          return createWorkspace(organizationId, owner.id, request);
        },
        autoCreateWorkspace: async ({
          templateName,
          versionId,
          organizationId,
          defaultBuildParameters,
          defaultName,
        }) => {
          let templateVersionParameters;
          if (versionId) {
            templateVersionParameters = { template_version_id: versionId };
          } else {
            const template = await getTemplateByName(
              organizationId,
              templateName,
            );
            templateVersionParameters = { template_id: template.id };
          }
          return createWorkspace(organizationId, "me", {
            ...templateVersionParameters,
            name: defaultName,
            rich_parameter_values: defaultBuildParameters,
          });
        },
        loadFormData: async ({ templateName, organizationId, versionId }) => {
          const [template, permissions] = await Promise.all([
            getTemplateByName(organizationId, templateName),
            checkCreateWSPermissions(organizationId),
          ]);

          const realizedVersionId = versionId ?? template.active_version_id;
          const [parameters] = await Promise.all([
            getTemplateVersionRichParameters(realizedVersionId).then((p) =>
              p.filter(paramsUsedToCreateWorkspace),
            ),
          ]);

          return {
            template,
            permissions,
            parameters,
            versionId: realizedVersionId,
          };
        },
      },
      actions: {
        assignFormData: assign((ctx, event) => {
          return {
            ...ctx,
            ...event.data,
          };
        }),
        assignError: assign({
          error: (_, event) => event.data,
        }),
        clearError: assign({
          error: (_) => undefined,
        }),
      },
    },
  );

const checkCreateWSPermissions = async (organizationId: string) => {
  // HACK: below, we pass in * for the owner_id, which is a hacky way of checking if the
  // current user can create a workspace on behalf of anyone within the org (only org owners should be able to do this).
  // This pattern should not be replicated outside of this narrow use case.
  const permissionsToCheck = {
    createWorkspaceForUser: {
      object: {
        resource_type: "workspace",
        organization_id: organizationId,
        owner_id: "*",
      },
      action: "create",
    },
  } as const;

  return checkAuthorization({
    checks: permissionsToCheck,
  }) as Promise<Record<keyof typeof permissionsToCheck, boolean>>;
};

export type CreateWSPermissions = Awaited<
  ReturnType<typeof checkCreateWSPermissions>
>;
