import {
  checkAuthorization,
  createWorkspace,
  getTemplateByName,
  getTemplateVersionGitAuth,
  getTemplateVersionRichParameters,
} from "api/api";
import {
  CreateWorkspaceRequest,
  Template,
  TemplateVersionGitAuth,
  TemplateVersionParameter,
  User,
  Workspace,
  WorkspaceBuildParameter,
} from "api/typesGenerated";
import { assign, createMachine } from "xstate";
import { paramsUsedToCreateWorkspace } from "utils/workspace";
import { REFRESH_GITAUTH_BROADCAST_CHANNEL } from "utils/gitAuth";

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
  gitAuth?: TemplateVersionGitAuth[];
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
  /** @xstate-layout N4IgpgJg5mDOIC5QGMBOYCGAXMB1A9qgNawAOGyYAyltmAHTIAWYyRAlgHZQCy+EYAMQBtAAwBdRKFL5Y7LO3ycpIAB6IATAGYN9UQBYArDsP6A7Pq1WNAGhABPRAE4AHPUOHRG7xoCMxswA2Mw0AX1C7NEwcAmIyCmpaHEYWNi5efiFhX0kkEBk5BSUVdQQ-M3onM19fUV8nfRdvJ0M7RzKdejMPVw0mpyctJ0DwyPQ6WJJySho6egwAVyx8AGFxhW5BCCUGLgA3fCIGKInCKYTZ5MXltej0hH38ZGxFTjFxd5UC+VeSxA8KlULIN9AYXC4zE42ogALT1eiBLSGbyBQIQvoufSuUYgE4xM7xGZJBjXVbrdKCMCoVCEeikAA22AAZoQALaMdZ4AnTRJzUm3F7cB6cA7PIpvCSfPLfcV-BBaFxOegKwzVRouUTdAxmaEIXwuQx6QyBPq+QKgg2iEYRXGcyaE3nJen4DAQdIAMTZABFsBgtjt6I8jhzoly4jzLgxna6Pd7fcLRS8lO8pdJZD9inlSsEtF0qvoNI0rUEBrqtJZ6BpAtqFaIvE4cXiw+ciXNo27uJ7UKyfbRKdTaQzmWyQ6dwxdifR27Hu72MAmnkmJR8JF907Ks4hFYEjZiLKJDL4jIWyxWqwZ9Poj74LBDG3buRO5uwIPShCsAEoAUQAggAVL8AH1cAAeQ-ABpKgAAUfxWL9U3yddfk3BBAkMJVFScOtvExcwml1VVDS8dVAkGFx6mNe9Q3tCNJzxdIaISf1OF2EVDmOB9x1bZJ6O4RjKAXMVXhTVdpSQzNQGzRF3F8bxLAPJxCw0VoHEQSFRARKpyyMJEWiMKixxbR0OLuPjH0ofsaVQOlGSwFlu1HfEuOMxyGPMsBBKXETcjTQpkMkxAtGrSpyytFxAjNPxvAI0RcycXwtE1boISw9DDHCG1OEyeA8ibfjjLXPyJLUWEEq6Uj9DRZSNAPdCIV1OFNXoBLkQ0QZzUxUQGxtPL3MjFJWA4bg+AEQqM2UFDy0rRLjBvcwGlBXxdWGZVsKCtry1MYwDKcoz+v5cluDGjcAvlRSER0MwrqPcKEqhVSEBCegNS8LQ5pmwidubB1+unTs41oY7-JK1DgtNDVPCutL9F1bQ3EU0ErSqiH0p6zi9snF83yB4rswhPRUWGIJ9SvFpdSMXxmshfxIvQxEwjR6i+row6oHynGJtO80lQGVFgnIsxwS8XVzR3bpC2GaszXMXwvvy-qmwgDm5VIndvCsOtIUqmpbAexVdESiwC1VJEEv0OXmbbF0IC-AdUGVlD4t0KsPHe+o6h1B6jA0zVYuNPotCCSEMtCIA */
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
              gitAuth: TemplateVersionGitAuth[];
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
          invoke: [
            {
              src: () => (callback) => {
                const channel = watchGitAuthRefresh(() => {
                  callback("REFRESH_GITAUTH");
                });
                return () => channel.close();
              },
            },
          ],
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
        loadFormData: async ({ templateName, organizationId }) => {
          const [template, permissions] = await Promise.all([
            getTemplateByName(organizationId, templateName),
            checkCreateWSPermissions(organizationId),
          ]);
          const [parameters, gitAuth] = await Promise.all([
            getTemplateVersionRichParameters(template.active_version_id).then(
              (p) => p.filter(paramsUsedToCreateWorkspace),
            ),
            getTemplateVersionGitAuth(template.active_version_id),
          ]);

          return {
            template,
            permissions,
            parameters,
            gitAuth,
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

export const watchGitAuthRefresh = (callback: () => void) => {
  const bc = new BroadcastChannel(REFRESH_GITAUTH_BROADCAST_CHANNEL);
  bc.addEventListener("message", callback);
  return bc;
};

export type CreateWSPermissions = Awaited<
  ReturnType<typeof checkCreateWSPermissions>
>;
