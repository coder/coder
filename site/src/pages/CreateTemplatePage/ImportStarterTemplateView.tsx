import { type FC } from "react";
import { useQuery } from "react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  templateVersionLogs,
  JobError,
  templateExamples,
  templateVersionVariables,
} from "api/queries/templates";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { useDashboard } from "modules/dashboard/useDashboard";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { CreateTemplateForm } from "./CreateTemplateForm";
import {
  firstVersionFromExample,
  getFormPermissions,
  newTemplate,
} from "./utils";
import { CreateTemplatePageViewProps } from "./types";

export const ImportStarterTemplateView: FC<CreateTemplatePageViewProps> = ({
  onCreateTemplate,
  onOpenBuildLogsDrawer,
  variablesSectionRef,
  error,
  isCreating,
}) => {
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

  const isJobError = error instanceof JobError;
  const templateVersionLogsQuery = useQuery({
    ...templateVersionLogs(isJobError ? error.version.id : ""),
    enabled: isJobError,
  });

  const missedVariables = useQuery({
    ...templateVersionVariables(isJobError ? error.version.id : ""),
    keepPreviousData: true,
    enabled:
      isJobError && error.job.error_code === "REQUIRED_TEMPLATE_VARIABLES",
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
      starterTemplate={templateExample!}
      variables={missedVariables.data}
      error={error}
      isSubmitting={isCreating}
      onCancel={() => navigate(-1)}
      jobError={isJobError ? error.job.error : undefined}
      logs={templateVersionLogsQuery.data}
      onSubmit={async (formData) => {
        await onCreateTemplate({
          organizationId,
          version: firstVersionFromExample(
            templateExample!,
            formData.user_variable_values,
          ),
          template: newTemplate(formData),
        });
      }}
    />
  );
};
