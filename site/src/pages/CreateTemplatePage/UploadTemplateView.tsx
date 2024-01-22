import { useQuery, useMutation } from "react-query";
import { useNavigate } from "react-router-dom";
import {
  templateVersionLogs,
  JobError,
  createTemplate,
  templateVersionVariables,
} from "api/queries/templates";
import { uploadFile } from "api/queries/files";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { CreateTemplateForm } from "./CreateTemplateForm";
import { firstVersionFromFile, getFormPermissions, newTemplate } from "./utils";

export const UploadTemplateView = () => {
  const navigate = useNavigate();
  const organizationId = useOrganizationId();

  const dashboard = useDashboard();
  const formPermissions = getFormPermissions(dashboard.entitlements);

  const uploadFileMutation = useMutation(uploadFile());
  const uploadedFile = uploadFileMutation.data;

  const createTemplateMutation = useMutation(createTemplate());
  const createError = createTemplateMutation.error;
  const isJobError = createError instanceof JobError;
  const templateVersionLogsQuery = useQuery({
    ...templateVersionLogs(isJobError ? createError.version.id : ""),
    enabled: isJobError,
  });

  const missedVariables = useQuery({
    ...templateVersionVariables(isJobError ? createError.version.id : ""),
    enabled:
      isJobError &&
      createError.job.error_code === "REQUIRED_TEMPLATE_VARIABLES",
  });

  return (
    <CreateTemplateForm
      {...formPermissions}
      variables={missedVariables.data}
      error={createTemplateMutation.error}
      isSubmitting={createTemplateMutation.isLoading}
      onCancel={() => navigate(-1)}
      jobError={isJobError ? createError.job.error : undefined}
      logs={templateVersionLogsQuery.data}
      upload={{
        onUpload: uploadFileMutation.mutateAsync,
        isUploading: uploadFileMutation.isLoading,
        onRemove: uploadFileMutation.reset,
        file: uploadFileMutation.variables,
      }}
      onSubmit={async (formData) => {
        const template = await createTemplateMutation.mutateAsync({
          organizationId,
          version: firstVersionFromFile(
            uploadedFile!.hash,
            formData.user_variable_values,
          ),
          template: newTemplate(formData),
        });
        navigate(`/templates/${template.name}`);
      }}
    />
  );
};
