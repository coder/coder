import { type FC } from "react";
import { useQuery, useMutation } from "react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  templateVersionLogs,
  templateByName,
  templateVersion,
  templateVersionVariables,
  JobError,
  createTemplate,
} from "api/queries/templates";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { useDashboard } from "modules/dashboard/useDashboard";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { CreateTemplateForm } from "./CreateTemplateForm";
import { firstVersionFromFile, getFormPermissions, newTemplate } from "./utils";
import { Template } from "api/typesGenerated";

type DuplicateTemplateViewProps = {
  onSuccess: (template: Template) => void;
};

export const DuplicateTemplateView: FC<DuplicateTemplateViewProps> = ({
  onSuccess,
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
        onSuccess(template);
      }}
    />
  );
};
