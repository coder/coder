import { getWorkspaceParameters, postWorkspaceBuild } from "api/api";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Loader } from "components/Loader/Loader";
import {
  WorkspaceParametersFormValues,
  WorkspaceParametersForm,
} from "./WorkspaceParametersForm";
import { useNavigate } from "react-router-dom";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { FC } from "react";
import { isApiValidationError } from "api/errors";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { WorkspaceBuildParameter } from "api/typesGenerated";

const WorkspaceParametersPage = () => {
  const workspace = useWorkspaceSettings();
  const parameters = useQuery({
    queryKey: ["workspace", workspace.id, "parameters"],
    queryFn: () => getWorkspaceParameters(workspace),
  });
  const navigate = useNavigate();
  const updateParameters = useMutation({
    mutationFn: (buildParameters: WorkspaceBuildParameter[]) =>
      postWorkspaceBuild(workspace.id, {
        transition: "start",
        rich_parameter_values: buildParameters,
      }),
    onSuccess: () => {
      navigate(`/${workspace.owner_name}/${workspace.name}`);
    },
  });

  return (
    <>
      <Helmet>
        <title>{pageTitle([workspace.name, "Parameters"])}</title>
      </Helmet>

      <WorkspaceParametersPageView
        data={parameters.data}
        submitError={updateParameters.error}
        isSubmitting={updateParameters.isLoading}
        onSubmit={(values) => {
          // When updating the parameters, the API does not accept immutable
          // values so we need to filter them
          const onlyMultableValues = parameters
            .data!.templateVersionRichParameters.filter((p) => p.mutable)
            .map(
              (p) =>
                values.rich_parameter_values.find((v) => v.name === p.name)!,
            );
          updateParameters.mutate(onlyMultableValues);
        }}
        onCancel={() => {
          navigate("../..");
        }}
      />
    </>
  );
};

export type WorkspaceParametersPageViewProps = {
  data: Awaited<ReturnType<typeof getWorkspaceParameters>> | undefined;
  submitError: unknown;
  isSubmitting: boolean;
  onSubmit: (formValues: WorkspaceParametersFormValues) => void;
  onCancel: () => void;
};

export const WorkspaceParametersPageView: FC<
  WorkspaceParametersPageViewProps
> = ({ data, submitError, isSubmitting, onSubmit, onCancel }) => {
  return (
    <>
      <PageHeader
        css={{
          paddingTop: 0,
        }}
      >
        <PageHeaderTitle>Workspace parameters</PageHeaderTitle>
      </PageHeader>

      {submitError && !isApiValidationError(submitError) && (
        <ErrorAlert error={submitError} sx={{ mb: 6 }} />
      )}

      {data ? (
        <WorkspaceParametersForm
          buildParameters={data.buildParameters}
          templateVersionRichParameters={data.templateVersionRichParameters}
          error={submitError}
          isSubmitting={isSubmitting}
          onSubmit={onSubmit}
          onCancel={onCancel}
        />
      ) : (
        <Loader />
      )}
    </>
  );
};

export default WorkspaceParametersPage;
