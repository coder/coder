import { type FC, useCallback, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { getUserParameters } from "api/api";
import { checkAuthorization } from "api/queries/authCheck";
import {
  richParameters,
  templateByName,
  templateVersionExternalAuth,
} from "api/queries/templates";
import { autoCreateWorkspace, createWorkspace } from "api/queries/workspaces";
import type {
  TemplateVersionParameter,
  UserParameter,
  Workspace,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useMe } from "contexts/auth/useMe";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useDashboard } from "modules/dashboard/useDashboard";
import { generateWorkspaceName } from "modules/workspaces/generateWorkspaceName";
import { pageTitle } from "utils/page";
import type { AutofillBuildParameter } from "utils/richParameters";
import { paramsUsedToCreateWorkspace } from "utils/workspace";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";
import { createWorkspaceChecks, type CreateWSPermissions } from "./permissions";

export const createWorkspaceModes = ["form", "auto", "duplicate"] as const;
export type CreateWorkspaceMode = (typeof createWorkspaceModes)[number];

export type ExternalAuthPollingState = "idle" | "polling" | "abandoned";

const CreateWorkspacePage: FC = () => {
  const organizationId = useOrganizationId();
  const { template: templateName } = useParams() as { template: string };
  const me = useMe();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const mode = getWorkspaceMode(searchParams);
  const customVersionId = searchParams.get("version") ?? undefined;
  const { experiments } = useDashboard();

  const defaultName = searchParams.get("name");

  const queryClient = useQueryClient();
  const autoCreateWorkspaceMutation = useMutation(
    autoCreateWorkspace(queryClient),
  );
  const createWorkspaceMutation = useMutation(createWorkspace(queryClient));

  const templateQuery = useQuery(templateByName(organizationId, templateName));

  const permissionsQuery = useQuery(
    checkAuthorization({
      checks: createWorkspaceChecks(organizationId),
    }),
  );
  const realizedVersionId =
    customVersionId ?? templateQuery.data?.active_version_id;
  const richParametersQuery = useQuery({
    ...richParameters(realizedVersionId ?? ""),
    enabled: realizedVersionId !== undefined,
  });
  const realizedParameters = richParametersQuery.data
    ? richParametersQuery.data.filter(paramsUsedToCreateWorkspace)
    : undefined;

  const {
    externalAuth,
    externalAuthPollingState,
    startPollingExternalAuth,
    isLoadingExternalAuth,
  } = useExternalAuth(realizedVersionId);

  const isLoadingFormData =
    templateQuery.isLoading ||
    permissionsQuery.isLoading ||
    richParametersQuery.isLoading;
  const loadFormDataError =
    templateQuery.error ?? permissionsQuery.error ?? richParametersQuery.error;

  const title = autoCreateWorkspaceMutation.isLoading
    ? "Creating workspace..."
    : "Create workspace";

  const onCreateWorkspace = useCallback(
    (workspace: Workspace) => {
      navigate(`/@${workspace.owner_name}/${workspace.name}`);
    },
    [navigate],
  );

  // Auto fill parameters
  const userParametersQuery = useQuery({
    queryKey: ["userParameters"],
    queryFn: () => getUserParameters(templateQuery.data!.id),
    enabled:
      experiments.includes("auto-fill-parameters") && templateQuery.isSuccess,
  });
  const autofillParameters = getAutofillParameters(
    searchParams,
    userParametersQuery.data ? userParametersQuery.data : [],
  );

  const automateWorkspaceCreation = useEffectEvent(async () => {
    try {
      const newWorkspace = await autoCreateWorkspaceMutation.mutateAsync({
        templateName,
        organizationId,
        defaultBuildParameters: autofillParameters,
        defaultName: defaultName ?? generateWorkspaceName(),
        versionId: realizedVersionId,
      });

      onCreateWorkspace(newWorkspace);
    } catch (err) {
      searchParams.delete("mode");
      setSearchParams(searchParams);
    }
  });

  useEffect(() => {
    if (mode === "auto") {
      void automateWorkspaceCreation();
    }
  }, [automateWorkspaceCreation, mode]);

  return (
    <>
      <Helmet>
        <title>{pageTitle(title)}</title>
      </Helmet>
      {loadFormDataError && <ErrorAlert error={loadFormDataError} />}
      {isLoadingFormData ||
      isLoadingExternalAuth ||
      autoCreateWorkspaceMutation.isLoading ? (
        <Loader />
      ) : (
        <CreateWorkspacePageView
          mode={mode}
          defaultName={defaultName}
          defaultOwner={me}
          autofillParameters={autofillParameters}
          error={createWorkspaceMutation.error}
          resetMutation={createWorkspaceMutation.reset}
          template={templateQuery.data!}
          versionId={realizedVersionId}
          externalAuth={externalAuth ?? []}
          externalAuthPollingState={externalAuthPollingState}
          startPollingExternalAuth={startPollingExternalAuth}
          permissions={permissionsQuery.data as CreateWSPermissions}
          parameters={realizedParameters as TemplateVersionParameter[]}
          creatingWorkspace={createWorkspaceMutation.isLoading}
          onCancel={() => {
            navigate(-1);
          }}
          onSubmit={async (request, owner) => {
            if (realizedVersionId) {
              request = {
                ...request,
                template_id: undefined,
                template_version_id: realizedVersionId,
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
  const [externalAuthPollingState, setExternalAuthPollingState] =
    useState<ExternalAuthPollingState>("idle");

  const startPollingExternalAuth = useCallback(() => {
    setExternalAuthPollingState("polling");
  }, []);

  const { data: externalAuth, isLoading: isLoadingExternalAuth } = useQuery(
    versionId
      ? {
          ...templateVersionExternalAuth(versionId),
          refetchInterval:
            externalAuthPollingState === "polling" ? 1000 : false,
        }
      : { enabled: false },
  );

  const allSignedIn = externalAuth?.every((it) => it.authenticated);

  useEffect(() => {
    if (allSignedIn) {
      setExternalAuthPollingState("idle");
      return;
    }

    if (externalAuthPollingState !== "polling") {
      return;
    }

    // Poll for a maximum of one minute
    const quitPolling = setTimeout(
      () => setExternalAuthPollingState("abandoned"),
      60_000,
    );
    return () => {
      clearTimeout(quitPolling);
    };
  }, [externalAuthPollingState, allSignedIn]);

  return {
    startPollingExternalAuth,
    externalAuth,
    externalAuthPollingState,
    isLoadingExternalAuth,
  };
};

const getAutofillParameters = (
  urlSearchParams: URLSearchParams,
  userParameters: UserParameter[],
): AutofillBuildParameter[] => {
  const userParamMap = userParameters.reduce((acc, param) => {
    acc.set(param.name, param);
    return acc;
  }, new Map<string, UserParameter>());

  const buildValues: AutofillBuildParameter[] = Array.from(
    urlSearchParams.keys(),
  )
    .filter((key) => key.startsWith("param."))
    .map((key) => {
      const name = key.replace("param.", "");
      const value = urlSearchParams.get(key) ?? "";
      // URL should take precedence over user parameters
      userParamMap.delete(name);
      return { name, value, source: "url" };
    });

  userParamMap.forEach((param) => {
    buildValues.push({
      name: param.name,
      value: param.value,
      source: "user_history",
    });
  });
  return buildValues;
};

export default CreateWorkspacePage;

function getWorkspaceMode(params: URLSearchParams): CreateWorkspaceMode {
  const paramMode = params.get("mode");
  if (createWorkspaceModes.includes(paramMode as CreateWorkspaceMode)) {
    return paramMode as CreateWorkspaceMode;
  }

  return "form";
}
