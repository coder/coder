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
  /** @xstate-layout N4IgpgJg5mDOIC5QGMBOYCGAXMB1A9qgNawAOGyYAyltmAHQxZYCWAdlACpgC2pANnVgBiCPjYN2AN3xEGaTDgLEyFarRyMwzdl14ChCafmTYW4gNoAGALrWbiUKXywWrcY5AAPRABYATAA0IACeiACMvgCs9ADssQBsVgDM-v5JyQnJUQkAvrnBCnTKJOSUNHRaOhzcfII4ImIS9MZy9EVKhKVqFZpMrDX69XBGbDKm7mz2FuEOSCDOrpOePgjJAJzr9AAcVr4J677bCeFWB3vBYQgBvvQJB-5R6+GxVulPUfmF6MVdquUaBj9XS1AwNYRgVCoQj0MEAM0IPHaP06KjK6kqwMGdUMxgm5imtnsnkWbgJK0QGy2u32h2Op3OvkuiH8vis9HCDxO0XC2z5CX8XxAHTwf3RvSB2gGehxOCoyAAFrwMKJxJIxrJ5CjRWieoCqtLQcN5UqeBhRuMzJYibYSS4yR55qtTv52bFeYlfPEor4XuFmddwlscqz1mdfMlfZlfEKRSV-hi+lKQUM6CblRCoTD4YjkYodd0AZjk9iwdRFcqLSYrYS7Lb5qTlk6Im83R6El7Yj6-QH4gl6M8ctsoht7skrJ8CsLtfHxfqsTKywAFDCoDA8bSQxpqloatpxsV64vVRfDFdrjc4VCwKv4611uZOe1N0DOgXhOKRjZf-axR4Btl2XWWJngSKJtgCHJwnSWMZ0PIskxPI06HPddN2vTNoVQWF6gRVAkQPXUEMlJDUxwVDLy3W8a2mesnyWclmwQTl-A-WIv3WH8Ej-KIAyyW4Em2Z5XV5fx1m48JYPzWcj0Qw0yLAABxNwAEEAFcsAVVVmlaLVpPgxMSPk2UlNUjSFWoyZaMfBZn0Y18WSDfsfUEvlfEHViA2SbZ-HoDZwPSe4omCwUp0IwtDINFMTOUrB1M0zDs1w3NwoTCUotLYZYviiy8Rom0bMbezvEc8T6BcvkII8-1Qj8dZfP2R4fXEziUliKTfiIyKK2QIhdCXSEeBYWBXHEbcdL3eQlV6gb8OG0a2FgYkGzsx0HIQfwfJiKJXmSZJoO88IRwAqwP22aD3OeLshP2DrUQi9Ker6jhZqGkaCRESEsJw7A8II6aiFe+aPuW+iHTYCkNqiKxtgHVi+XWYK2SZWqNr5HZslAiNoJSSdvn0rr0rhFh+H4frV3XEQAGEACUAFEVM4OmAH1cAAeRpgBpKglxUqm6dB2yGLWkq0cecrdv2-xDuO1GXluRH9s5H1wKObi7oLNL9WJ0nyYvEQqDpgAZOmqc4Zm2dwAA5OmacFoqRdWcd0asSWkZuoJUbSftpc2vZ6rZDsXg1mTiLzMwOFDsBtPVGR9zgwn9Q6XQo8sglrLtYWIaYzbxZ2lIpZl5IA3O+hLvWeleSiVj6pDgzHpRFODMS7Cc3w8P7q1ypk8jgy0-ve3Vuz9bc+2yWDvO2WrjE2HMcybZYiOHJYm2fIpzYfAIDgTxUrnOhM-ByGAFoEgDE-6CsS+3n8d0nl9aW8enAmHvnEtTyEA+X1F4cOWryM0k5JyTiAZWS3DSKBdIC9zjZDronY8xkyzpjNJ-YqqxXjORXuEfaEYAh9gAtLO4aQNgenqkdSMsCX7wOisuCmlFrwoMdhETazl9hCQ2NLKwFd1gnRiCkMM51og8k2BQruclqFZTMppBhw9RZBldvQTa0FzgdnuF5Y4ZdgrumyI8JyC8RF700E9fqg1gZjWkZDUBWwDgjjElgnawE1GxAUUJPYglwIjjeDGMKCdKGaB1mTF6tD4ArSzpDfabwdiXxApseqtjT6o1SE4txrVXTpHSF4-GnVfF6QjlAKO5ic5RCOn5LsbwjpHWeJyAMZVThHUgvcCSvp9GyRyTgCABT1rhN8rsV2MTYmgQDC6BR7xuznByCcZpYcvqEA6aLSxdxFa2OyCBWIAYimwxXvwoMrp3GrzXkAA */
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
        createWorkspace: ({ organizationId }, { request, owner }) => {
          return createWorkspace(organizationId, owner.id, request);
        },
        autoCreateWorkspace: async ({
          templateName,
          organizationId,
          defaultBuildParameters,
          defaultName,
        }) => {
          const template = await getTemplateByName(
            organizationId,
            templateName,
          );
          return createWorkspace(organizationId, "me", {
            template_id: template.id,
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
