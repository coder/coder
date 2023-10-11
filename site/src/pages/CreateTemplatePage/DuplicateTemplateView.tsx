import { useQuery, useMutation } from "react-query";
import { templateVersionLogs } from "api/queries/templateVersions";
import {
  templateByName,
  templateVersion,
  templateVersionVariables,
  JobError,
  createTemplate,
} from "api/queries/templates";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { useOrganizationId } from "hooks";
import { useNavigate, useSearchParams } from "react-router-dom";
import { CreateTemplateForm } from "./CreateTemplateForm";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { firstVersionFromFile, getFormPermissions, newTemplate } from "./utils";

export const DuplicateTemplateView = () => {
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

  const createTemplateMutation = useMutation(createTemplate());
  const createError = createTemplateMutation.error;
  const isJobError = createError instanceof JobError;
  const templateVersionLogsQuery = useQuery({
    ...templateVersionLogs(isJobError ? createError.version.id : ""),
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
      copiedTemplate={templateByNameQuery.data!}
      error={createTemplateMutation.error}
      isSubmitting={createTemplateMutation.isLoading}
      variables={templateVersionVariablesQuery.data}
      onCancel={() => navigate(-1)}
      jobError={isJobError ? createError.job.error : undefined}
      logs={templateVersionLogsQuery.data}
      onSubmit={async (formData) => {
        const template = await createTemplateMutation.mutateAsync({
          organizationId,
          version: firstVersionFromFile(
            templateVersionQuery.data!.job.file_id,
            formData.user_variable_values,
          ),
          template: newTemplate(formData),
        });
        navigate(`/templates/${template.name}`);
      }}
    />
  );
};
