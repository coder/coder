import { useMachine } from "@xstate/react";
import { WorkspaceBuildParameter } from "api/typesGenerated";
import { useMe } from "hooks/useMe";
import { useOrganizationId } from "hooks/useOrganizationId";
import { type FC, useState, useEffect } from "react";
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
import { useQuery } from "react-query";
import { templateVersionExternalAuth } from "api/queries/templates";

export type ExternalAuthPollingState = "idle" | "polling" | "abandoned";

const CreateWorkspacePage: FC = () => {
  const organizationId = useOrganizationId();
  const [searchParams] = useSearchParams();
  const { template: templateName } = useParams() as { template: string };
  const me = useMe();
  const navigate = useNavigate();

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

  const [externalAuthPollingState, setExternalAuthPollingState] =
    useState<ExternalAuthPollingState>("idle");

  const { data: externalAuth, error } = useQuery(
    versionId
      ? {
          ...templateVersionExternalAuth(versionId),
          refetchInterval:
            externalAuthPollingState === "polling" ? 1000 : false,
        }
      : { enabled: false },
  );

  useEffect(() => {
    if (externalAuthPollingState !== "polling") {
      return;
    }

    const pollingTimeoutId = window.setTimeout(
      () => setExternalAuthPollingState("abandoned"),
      60_000,
    );

    return () => window.clearTimeout(pollingTimeoutId);
  }, [externalAuthPollingState]);

  if (externalAuthPollingState !== "idle") {
    const allSignedIn = externalAuth?.every((it) => it.authenticated) ?? false;
    if (allSignedIn) {
      setExternalAuthPollingState("idle");
    }
  }

  return (
    <>
      <Helmet>
        <title>
          {pageTitle(
            createWorkspaceState.matches("autoCreating")
              ? "Creating workspace..."
              : "Create workspace",
          )}
        </title>
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
          permissions={permissions as CreateWSPermissions}
          parameters={parameters!}
          creatingWorkspace={createWorkspaceState.matches("creatingWorkspace")}
          startPollingExternalAuth={() => {
            setExternalAuthPollingState("polling");
          }}
          onCancel={() => navigate(-1)}
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
  return Array.from(urlSearchParams.keys())
    .filter((key) => key.startsWith("param."))
    .map((key) => {
      const name = key.replace("param.", "");
      const value = urlSearchParams.get(key) ?? "";
      return { name, value };
    });
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
