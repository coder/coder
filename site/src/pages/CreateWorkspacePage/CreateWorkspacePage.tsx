import {
  TemplateVersionParameter,
  Workspace,
  WorkspaceBuildParameter,
} from "api/typesGenerated";
import { useMe } from "hooks/useMe";
import { useOrganizationId } from "hooks/useOrganizationId";
import { type FC, useCallback, useState, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";
import { Loader } from "components/Loader/Loader";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  uniqueNamesGenerator,
  animals,
  colors,
  NumberDictionary,
} from "unique-names-generator";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
  templateByName,
  templateVersionExternalAuth,
  richParameters,
} from "api/queries/templates";
import {
  AutoCreateWorkspaceOptions,
  autoCreateWorkspace,
  createWorkspace,
} from "api/queries/workspaces";
import { checkAuthorization } from "api/queries/authCheck";
import { CreateWSPermissions, createWorkspaceChecks } from "./permissions";
import { useEffectEvent } from "hooks/hookPolyfills";
import { workspaceBuildParameters } from "api/queries/workspaceBuilds";

export const createWorkspaceModes = ["form", "auto", "duplicate"] as const;
export type CreateWorkspaceMode = (typeof createWorkspaceModes)[number];

export type ExternalAuthPollingStatus = "idle" | "polling" | "abandoned";

const CreateWorkspacePage: FC = () => {
  const organizationId = useOrganizationId();
  const [searchParams] = useSearchParams();
  const { template: templateName } = useParams() as { template: string };
  const navigate = useNavigate();
  const me = useMe();

  const queryClient = useQueryClient();
  const createWorkspaceMutation = useMutation(createWorkspace(queryClient));

  const templateQuery = useQuery(templateByName(organizationId, templateName));
  const permissionsQuery = useQuery(
    checkAuthorization({ checks: createWorkspaceChecks(organizationId) }),
  );

  const versionId =
    searchParams.get("version") ?? templateQuery.data?.active_version_id;

  const { authList, pollingStatus, startPolling } = useExternalAuth(versionId);
  const richParametersQuery = useQuery({
    ...richParameters(versionId ?? ""),
    enabled: versionId !== undefined,
  });

  const defaultBuildParameters = getDefaultBuildParameters(searchParams);
  const mode = getWorkspaceMode(searchParams);
  const defaultName = getDefaultName(mode, searchParams);

  const onCreateWorkspace = (workspace: Workspace) => {
    navigate(`/@${workspace.owner_name}/${workspace.name}`);
  };

  const isAutoCreating = useAutomatedWorkspaceCreation({
    auto: mode === "auto",
    onSuccess: onCreateWorkspace,
    payload: {
      templateName,
      organizationId,
      defaultBuildParameters,
      defaultName,
      versionId,
    },
  });

  const isLoadingFormData =
    templateQuery.isLoading ||
    permissionsQuery.isLoading ||
    richParametersQuery.isLoading;

  const loadFormDataError =
    templateQuery.error ?? permissionsQuery.error ?? richParametersQuery.error;

  return (
    <>
      <Helmet>
        <title>
          {pageTitle(
            isAutoCreating ? "Creating workspace..." : "Create workspace",
          )}
        </title>
      </Helmet>

      {loadFormDataError && <ErrorAlert error={loadFormDataError} />}

      {isLoadingFormData || isAutoCreating ? (
        <Loader />
      ) : (
        <CreateWorkspacePageView
          mode={mode}
          defaultName={defaultName}
          defaultOwner={me}
          defaultBuildParameters={defaultBuildParameters}
          error={createWorkspaceMutation.error}
          template={templateQuery.data!}
          versionId={versionId}
          externalAuth={authList ?? []}
          externalAuthPollingStatus={pollingStatus}
          startPollingExternalAuth={startPolling}
          permissions={permissionsQuery.data as CreateWSPermissions}
          creatingWorkspace={createWorkspaceMutation.isLoading}
          onCancel={() => navigate(-1)}
          parameters={richParametersQuery.data!.filter(
            (param) => !param.ephemeral,
          )}
          onSubmit={async (request, owner) => {
            if (versionId) {
              request = {
                ...request,
                template_id: undefined,
                template_version_id: versionId,
              };
            }

            const workspace = await createWorkspaceMutation.mutateAsync({
              ...request,
              userId: owner.id,
              organizationId,
            });
            onCreateWorkspace(workspace);
          }}
        />
      )}
    </>
  );
};

const useExternalAuth = (versionId: string | undefined) => {
  const [pollingStatus, setPollingStatus] =
    useState<ExternalAuthPollingStatus>("idle");

  const startPolling = useCallback(() => {
    setPollingStatus("polling");
  }, []);

  const { data: authList } = useQuery(
    versionId
      ? {
          ...templateVersionExternalAuth(versionId),
          refetchInterval: pollingStatus === "polling" ? 1000 : false,
        }
      : { enabled: false },
  );

  useEffect(() => {
    if (pollingStatus !== "polling") {
      return;
    }

    const timeoutId = window.setTimeout(
      () => setPollingStatus("abandoned"),
      60_000,
    );

    return () => clearTimeout(timeoutId);
  }, [pollingStatus]);

  const isAllSignedIn = authList?.every((it) => it.authenticated) ?? false;

  // Doing state sync inline, because doing it inside a useEffect call would add
  // unnecessary renders and re-painting.
  if (isAllSignedIn && pollingStatus === "polling") {
    setPollingStatus("idle");
  }

  return { authList, isAllSignedIn, pollingStatus, startPolling } as const;
};

function getWorkspaceMode(params: URLSearchParams): CreateWorkspaceMode {
  const paramMode = params.get("mode");
  if (createWorkspaceModes.includes(paramMode as CreateWorkspaceMode)) {
    return paramMode as CreateWorkspaceMode;
  }

  return "form";
}

function getDefaultName(mode: CreateWorkspaceMode, params: URLSearchParams) {
  if (mode === "auto") {
    return generateUniqueName();
  }

  const paramsName = params.get("name");
  if (mode === "duplicate" && paramsName) {
    return `${paramsName}-copy`;
  }

  return paramsName ?? "";
}

type AutomatedWorkspaceConfig = {
  auto: boolean;
  payload: AutoCreateWorkspaceOptions;
  onSuccess: (newWorkspace: Workspace) => void;
};

function useAutomatedWorkspaceCreation(config: AutomatedWorkspaceConfig) {
  // Duplicates some of the hook calls from the parent, but that was preferable
  // to having the function arguments balloon in complexity
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const autoCreateWorkspaceMutation = useMutation(
    autoCreateWorkspace(queryClient),
  );

  const automateWorkspaceCreation = useEffectEvent(async () => {
    try {
      const newWorkspace = await autoCreateWorkspaceMutation.mutateAsync(
        config.payload,
      );

      config.onSuccess(newWorkspace);
    } catch (err) {
      searchParams.delete("mode");
      setSearchParams(searchParams);
    }
  });

  useEffect(() => {
    if (config.auto) {
      void automateWorkspaceCreation();
    }
  }, [automateWorkspaceCreation, config.auto]);

  return autoCreateWorkspaceMutation.isLoading;
}

const getDefaultBuildParameters = (
  urlSearchParams: URLSearchParams,
): WorkspaceBuildParameter[] => {
  return [...urlSearchParams.keys()]
    .filter((key) => key.startsWith("param."))
    .map((key) => {
      return {
        name: key.replace("param.", ""),
        value: urlSearchParams.get(key) ?? "",
      };
    });
};

function getDuplicationUrlParams(
  workspaceParams: readonly WorkspaceBuildParameter[],
  workspace: Workspace,
): URLSearchParams {
  // Record type makes sure that every property key added starts with "param.";
  // page is also set up to parse params with this prefix for auto mode
  const consolidatedParams: Record<`param.${string}`, string> = {};

  for (const p of workspaceParams) {
    consolidatedParams[`param.${p.name}`] = p.value;
  }

  return new URLSearchParams({
    ...consolidatedParams,
    mode: "duplicate" satisfies CreateWorkspaceMode,
    name: workspace.name,
    version: workspace.template_active_version_id,
  });
}

/**
 * Takes a workspace, and returns out a function that will navigate the user to
 * the 'Create Workspace' page, pre-filling the form with as much information
 * about the workspace as possible.
 */
// Meant to be consumed by components outside of this file
export function useWorkspaceDuplication(workspace: Workspace) {
  const navigate = useNavigate();
  const buildParametersQuery = useQuery(
    workspaceBuildParameters(workspace.latest_build.id),
  );

  // Not using useEffectEvent for this, even with the slightly more complicated
  // dependency array, because useEffect isn't really an intended use case
  const duplicateWorkspace = useCallback(() => {
    const buildParams = buildParametersQuery.data;
    if (buildParams === undefined) {
      return;
    }

    const newUrlParams = getDuplicationUrlParams(buildParams, workspace);

    // Necessary for giving modals/popups time to flush their state changes and
    // close the popup before actually navigating. Otherwise, you risk the modal
    // awkwardly hanging there during the page transition
    void Promise.resolve().then(() => {
      navigate({
        pathname: `/templates/${workspace.template_name}/workspace`,
        search: newUrlParams.toString(),
      });
    });
  }, [navigate, workspace, buildParametersQuery.data]);

  return {
    duplicateWorkspace,
    duplicationReady: !buildParametersQuery.isLoading,
  } as const;
}

export const orderTemplateParameters = (
  templateParameters?: readonly TemplateVersionParameter[],
) => {
  return {
    mutable: templateParameters?.filter((p) => p.mutable) ?? [],
    immutable: templateParameters?.filter((p) => !p.mutable) ?? [],
  } as const;
};

const generateUniqueName = () => {
  const numberDictionary = NumberDictionary.generate({ min: 0, max: 99 });
  return uniqueNamesGenerator({
    dictionaries: [colors, animals, numberDictionary],
    separator: "-",
    length: 3,
    style: "lowerCase",
  });
};

export default CreateWorkspacePage;
