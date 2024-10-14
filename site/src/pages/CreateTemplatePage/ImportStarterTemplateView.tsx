import {
	JobError,
	templateExamples,
	templateVersionLogs,
	templateVersionVariables,
} from "api/queries/templates";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { CreateTemplateForm } from "./CreateTemplateForm";
import type { CreateTemplatePageViewProps } from "./types";
import {
	firstVersionFromExample,
	getFormPermissions,
	newTemplate,
} from "./utils";

export const ImportStarterTemplateView: FC<CreateTemplatePageViewProps> = ({
	onCreateTemplate,
	onOpenBuildLogsDrawer,
	variablesSectionRef,
	error,
	isCreating,
}) => {
	const navigate = useNavigate();
	const { entitlements, showOrganizations } = useDashboard();
	const [searchParams] = useSearchParams();
	const templateExamplesQuery = useQuery(templateExamples());
	const templateExample = templateExamplesQuery.data?.find(
		(e) => e.id === searchParams.get("exampleId")!,
	);

	const isLoading = templateExamplesQuery.isLoading;
	const loadingError = templateExamplesQuery.error;

	const formPermissions = getFormPermissions(entitlements);

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
			showOrganizationPicker={showOrganizations}
			onSubmit={async (formData) => {
				await onCreateTemplate({
					organization: formData.organization,
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
