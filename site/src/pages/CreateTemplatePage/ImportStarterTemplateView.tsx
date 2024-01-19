import { type FC } from "react";
import { useQuery, useMutation } from "react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  templateVersionLogs,
  JobError,
  createTemplate,
  templateExamples,
  templateVersionVariables,
} from "api/queries/templates";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { CreateTemplateForm } from "./CreateTemplateForm";
import {
  firstVersionFromExample,
  getFormPermissions,
  newTemplate,
} from "./utils";

export const ImportStarterTemplateView: FC = () => {
  const navigate = useNavigate();
  const organizationId = useOrganizationId();
  const [searchParams] = useSearchParams();
  const templateExamplesQuery = useQuery(templateExamples(organizationId));
  const templateExample = templateExamplesQuery.data?.find(
    (e) => e.id === searchParams.get("exampleId")!,
  );

  const isLoading = templateExamplesQuery.isLoading;
  const loadingError = templateExamplesQuery.error;

  const dashboard = useDashboard();
  const formPermissions = getFormPermissions(dashboard.entitlements);

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

  if (isLoading) {
    return <Loader />;
  }

  if (loadingError) {
    return <ErrorAlert error={loadingError} />;
  }

  return (
    <CreateTemplateForm
      {...formPermissions}
      starterTemplate={templateExample!}
      variables={missedVariables.data}
      error={createTemplateMutation.error}
      isSubmitting={createTemplateMutation.isLoading}
      onCancel={() => navigate(-1)}
      jobError={isJobError ? createError.job.error : undefined}
      logs={templateVersionLogsQuery.data}
      onSubmit={async (formData) => {
        const template = await createTemplateMutation.mutateAsync({
          organizationId,
          version: firstVersionFromExample(
            templateExample!,
            formData.user_variable_values,
          ),
          template: newTemplate(formData),
        });
        navigate(`/templates/${template.name}`);
      }}
    />
  );
};
