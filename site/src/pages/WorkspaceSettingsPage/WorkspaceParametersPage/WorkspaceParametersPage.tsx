import Button from "@mui/material/Button";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import { Helmet } from "react-helmet-async";
import { getWorkspaceParameters, postWorkspaceBuild } from "api/api";
import { EmptyState } from "components/EmptyState/EmptyState";
import { pageTitle } from "utils/page";
import {
  WorkspacePermissions,
  workspaceChecks,
} from "../../WorkspacePage/permissions";
import { checkAuthorization } from "api/queries/authCheck";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import { templateByName } from "api/queries/templates";
import { useMutation, useQuery } from "react-query";
import { Loader } from "components/Loader/Loader";
import {
  type WorkspaceParametersFormValues,
  WorkspaceParametersForm,
} from "./WorkspaceParametersForm";
import { useNavigate } from "react-router-dom";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { type FC } from "react";
import { isApiValidationError } from "api/errors";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import type { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { docs } from "utils/docs";

const WorkspaceParametersPage: FC = () => {
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

  const templateQuery = useQuery({
    ...templateByName(workspace.organization_id, workspace.template_name ?? ""),
    enabled: workspace !== undefined,
  });
  const template = templateQuery.data;

  // Permissions
  const checks =
    workspace && template ? workspaceChecks(workspace, template) : {};
  const permissionsQuery = useQuery({
    ...checkAuthorization({ checks }),
    enabled: workspace !== undefined && template !== undefined,
  });
  const permissions = permissionsQuery.data as WorkspacePermissions | undefined;
  const canChangeVersions = Boolean(permissions?.updateTemplate);

  return (
    <>
      <Helmet>
        <title>{pageTitle([workspace.name, "Parameters"])}</title>
      </Helmet>

      <WorkspaceParametersPageView
        workspace={workspace}
        canChangeVersions={canChangeVersions}
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
  workspace: Workspace;
  canChangeVersions: boolean;
  data: Awaited<ReturnType<typeof getWorkspaceParameters>> | undefined;
  submitError: unknown;
  isSubmitting: boolean;
  onSubmit: (formValues: WorkspaceParametersFormValues) => void;
  onCancel: () => void;
};

export const WorkspaceParametersPageView: FC<
  WorkspaceParametersPageViewProps
> = ({
  workspace,
  canChangeVersions,
  data,
  submitError,
  onSubmit,
  isSubmitting,
  onCancel,
}) => {
  return (
    <>
      <PageHeader css={{ paddingTop: 0 }}>
        <PageHeaderTitle>Workspace parameters</PageHeaderTitle>
      </PageHeader>

      {submitError && !isApiValidationError(submitError) && (
        <ErrorAlert error={submitError} css={{ marginBottom: 48 }} />
      )}

      {data ? (
        data.templateVersionRichParameters.length > 0 ? (
          <WorkspaceParametersForm
            workspace={workspace}
            canChangeVersions={canChangeVersions}
            autofillParams={data.buildParameters.map((p) => ({
              ...p,
              source: "active_build",
            }))}
            templateVersionRichParameters={data.templateVersionRichParameters}
            error={submitError}
            isSubmitting={isSubmitting}
            onSubmit={onSubmit}
            onCancel={onCancel}
          />
        ) : (
          <EmptyState
            message="This workspace has no parameters"
            cta={
              <Button
                component="a"
                href={docs("/templates/parameters")}
                startIcon={<OpenInNewOutlined />}
                variant="contained"
                target="_blank"
                rel="noreferrer"
              >
                Learn more about parameters
              </Button>
            }
            css={(theme) => ({
              border: `1px solid ${theme.palette.divider}`,
              borderRadius: 8,
            })}
          />
        )
      ) : (
        <Loader />
      )}
    </>
  );
};

export default WorkspaceParametersPage;
