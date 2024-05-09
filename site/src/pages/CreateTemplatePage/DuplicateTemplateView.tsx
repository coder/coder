import type { FC } from "react";
import { useQuery } from "react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  templateVersionLogs,
  templateByName,
  templateVersion,
  templateVersionVariables,
  JobError,
} from "api/queries/templates";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { CreateTemplateForm } from "./CreateTemplateForm";
import type { CreateTemplatePageViewProps } from "./types";
import { firstVersionFromFile, getFormPermissions, newTemplate } from "./utils";

export const DuplicateTemplateView: FC<CreateTemplatePageViewProps> = ({
  onCreateTemplate,
  onOpenBuildLogsDrawer,
  variablesSectionRef,
  error,
  isCreating,
}) => {
  const navigate = useNavigate();
  const { organizationId } = useAuthenticated();
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
            formData.provisioner_type,
          ),
          template: newTemplate(formData),
        });
      }}
    />
  );
};
