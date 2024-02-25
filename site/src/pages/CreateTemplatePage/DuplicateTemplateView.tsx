import { type FC } from "react";
import { useQuery } from "react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  templateVersionLogs,
  templateByName,
  templateVersion,
  templateVersionVariables,
  JobError,
} from "api/queries/templates";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { useDashboard } from "modules/dashboard/useDashboard";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { CreateTemplateForm } from "./CreateTemplateForm";
import { firstVersionFromFile, getFormPermissions, newTemplate } from "./utils";
import { CreateTemplatePageViewProps } from "./types";

export const DuplicateTemplateView: FC<CreateTemplatePageViewProps> = ({
  onCreateTemplate,
  onOpenBuildLogsDrawer,
  variablesSectionRef,
  error,
  isCreating,
}) => {
  const navigate = useNavigate();
  const organizationId = useOrganizationId();
  const [searchParams] = useSearchParams();
  const templateByNameQuery = useQuery(
    templateByName(organizationId, searchParams.get("fromTemplate")!),
  );
  const activeVersionId = templateByNameQuery.data?.active_version_id ?? "";
  const templateVersionQuery = useQuery({
    ...templateVersion(activeVersionId),
    enabled: templateByNameQuery.isSuccess,
  });
  const templateVersionVariablesQuery = useQuery({
    ...templateVersionVariables(activeVersionId),
    enabled: templateByNameQuery.isSuccess,
  });
  const isLoading =
    templateByNameQuery.isLoading ||
    templateVersionQuery.isLoading ||
    templateVersionVariablesQuery.isLoading;
  const loadingError =
    templateByNameQuery.error ||
    templateVersionQuery.error ||
    templateVersionVariablesQuery.error;

  const dashboard = useDashboard();
  const formPermissions = getFormPermissions(dashboard.entitlements);

  const isJobError = error instanceof JobError;
  const templateVersionLogsQuery = useQuery({
    ...templateVersionLogs(isJobError ? error.version.id : ""),
    enabled: isJobError,
  });

  if (isLoading) {
    return <Loader />;
  }

  if (loadingError) {
    return <ErrorAlert error={loadingError} />;
  }

  return (
    <CreateTemplateForm
      {...formPermissions}
      variablesSectionRef={variablesSectionRef}
      onOpenBuildLogsDrawer={onOpenBuildLogsDrawer}
      copiedTemplate={templateByNameQuery.data!}
      error={error}
      isSubmitting={isCreating}
      variables={templateVersionVariablesQuery.data}
      onCancel={() => navigate(-1)}
      jobError={isJobError ? error.job.error : undefined}
      logs={templateVersionLogsQuery.data}
      onSubmit={async (formData) => {
        await onCreateTemplate({
          organizationId,
          version: firstVersionFromFile(
            templateVersionQuery.data!.job.file_id,
            formData.user_variable_values,
          ),
          template: newTemplate(formData),
        });
      }}
    />
  );
};
