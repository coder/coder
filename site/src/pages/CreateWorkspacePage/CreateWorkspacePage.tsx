import { useMachine } from "@xstate/react";
import {
  Template,
  TemplateVersionGitAuth,
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated";
import { useMe } from "hooks/useMe";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
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
    },
    actions: {
      onCreateWorkspace: (_, event) => {
        navigate(`/@${event.data.owner_name}/${event.data.name}`);
      },
    },
  });
  const { template, error, parameters, permissions, gitAuth, defaultName } =
    createWorkspaceState.context;
  const title = createWorkspaceState.matches("autoCreating")
    ? "Creating workspace..."
    : "Create Workspace";

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
          template={template as Template}
          gitAuth={gitAuth as TemplateVersionGitAuth[]}
          permissions={permissions as CreateWSPermissions}
          parameters={parameters as TemplateVersionParameter[]}
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
