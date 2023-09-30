import { useMachine } from "@xstate/react";
import {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated";
import { useMe } from "hooks/useMe";
import { useOrganizationId } from "hooks/useOrganizationId";
import { type FC, useCallback, useState, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import {
  CreateWSPermissions,
  CreateWorkspaceMode,
  createWorkspaceMachine,
} from "xServices/createWorkspace/createWorkspaceXService";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";
import { Loader } from "components/Loader/Loader";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  uniqueNamesGenerator,
  animals,
  colors,
  NumberDictionary,
} from "unique-names-generator";
import { useQuery } from "@tanstack/react-query";
import { templateVersionExternalAuth } from "api/queries/templates";

export type ExternalAuthPollingState = "idle" | "polling" | "abandoned";

const CreateWorkspacePage: FC = () => {
  const organizationId = useOrganizationId();
  const { template: templateName } = useParams() as { template: string };
  const me = useMe();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const defaultBuildParameters = getDefaultBuildParameters(searchParams);
  const mode = (searchParams.get("mode") ?? "form") as CreateWorkspaceMode;
  const [createWorkspaceState, send] = useMachine(createWorkspaceMachine, {
    context: {
      organizationId,
      templateName,
      mode,
      defaultBuildParameters,
      defaultName:
        mode === "auto" ? generateUniqueName() : searchParams.get("name") ?? "",
      versionId: searchParams.get("version") ?? undefined,
    },
    actions: {
      onCreateWorkspace: (_, event) => {
        navigate(`/@${event.data.owner_name}/${event.data.name}`);
      },
    },
  });
  const { template, parameters, permissions, defaultName, versionId } =
    createWorkspaceState.context;
  const title = createWorkspaceState.matches("autoCreating")
    ? "Creating workspace..."
    : "Create workspace";

  const [externalAuthPollingState, setExternalAuthPollingState] =
    useState<ExternalAuthPollingState>("idle");

  const startPollingExternalAuth = useCallback(() => {
    setExternalAuthPollingState("polling");
  }, []);

  const { data: externalAuth, error } = useQuery(
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

  return (
    <>
      <Helmet>
        <title>{pageTitle(title)}</title>
      </Helmet>
      {Boolean(
        createWorkspaceState.matches("loadingFormData") ||
          createWorkspaceState.matches("autoCreating"),
      ) && <Loader />}
      {createWorkspaceState.matches("loadError") && (
        <ErrorAlert error={error} />
      )}
      {createWorkspaceState.matches("idle") && (
        <CreateWorkspacePageView
          defaultName={defaultName}
          defaultOwner={me}
          defaultBuildParameters={defaultBuildParameters}
          error={error}
          template={template!}
          versionId={versionId}
          externalAuth={externalAuth ?? []}
          externalAuthPollingState={externalAuthPollingState}
          startPollingExternalAuth={startPollingExternalAuth}
          permissions={permissions as CreateWSPermissions}
          parameters={parameters!}
          creatingWorkspace={createWorkspaceState.matches("creatingWorkspace")}
          onCancel={() => {
            navigate(-1);
          }}
          onSubmit={(request, owner) => {
            send({
              type: "CREATE_WORKSPACE",
              request,
              owner,
            });
          }}
        />
      )}
    </>
  );
};

export default CreateWorkspacePage;

const getDefaultBuildParameters = (
  urlSearchParams: URLSearchParams,
): WorkspaceBuildParameter[] => {
  const buildValues: WorkspaceBuildParameter[] = [];
  Array.from(urlSearchParams.keys())
    .filter((key) => key.startsWith("param."))
    .forEach((key) => {
      const name = key.replace("param.", "");
      const value = urlSearchParams.get(key) ?? "";
      buildValues.push({ name, value });
    });
  return buildValues;
};

export const orderedTemplateParameters = (
  templateParameters?: TemplateVersionParameter[],
): TemplateVersionParameter[] => {
  if (!templateParameters) {
    return [];
  }

  const immutables = templateParameters.filter(
    (parameter) => !parameter.mutable,
  );
  const mutables = templateParameters.filter((parameter) => parameter.mutable);
  return [...immutables, ...mutables];
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
