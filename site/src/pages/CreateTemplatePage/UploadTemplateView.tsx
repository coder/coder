import { uploadFile } from "api/queries/files";
import {
	JobError,
	templateVersionLogs,
	templateVersionVariables,
} from "api/queries/templates";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { useMutation, useQuery } from "react-query";
import { useNavigate } from "react-router-dom";
import { CreateTemplateForm } from "./CreateTemplateForm";
import type { CreateTemplatePageViewProps } from "./types";
import { firstVersionFromFile, getFormPermissions, newTemplate } from "./utils";

export const UploadTemplateView: FC<CreateTemplatePageViewProps> = ({
	onCreateTemplate,
	onOpenBuildLogsDrawer,
	variablesSectionRef,
	isCreating,
	error,
}) => {
	const navigate = useNavigate();
	const { entitlements, showOrganizations } = useDashboard();
	const formPermissions = getFormPermissions(entitlements);

	const uploadFileMutation = useMutation(uploadFile());
	const uploadedFile = uploadFileMutation.data;

	const isJobError = error instanceof JobError;
	const templateVersionLogsQuery = useQuery({
		...templateVersionLogs(isJobError ? error.version.id : ""),
		enabled: isJobError,
	});

	const missedVariables = useQuery({
		...templateVersionVariables(isJobError ? error.version.id : ""),
		enabled:
			isJobError && error.job.error_code === "REQUIRED_TEMPLATE_VARIABLES",
	});

	return (
		<CreateTemplateForm
			{...formPermissions}
			onOpenBuildLogsDrawer={onOpenBuildLogsDrawer}
			variablesSectionRef={variablesSectionRef}
			variables={missedVariables.data}
			error={error}
			isSubmitting={isCreating}
			onCancel={() => navigate(-1)}
			jobError={isJobError ? error.job.error : undefined}
			logs={templateVersionLogsQuery.data}
			upload={{
				onUpload: uploadFileMutation.mutateAsync,
				isUploading: uploadFileMutation.isLoading,
				onRemove: uploadFileMutation.reset,
				file: uploadFileMutation.variables,
			}}
			showOrganizationPicker={showOrganizations}
			onSubmit={async (formData) => {
				await onCreateTemplate({
					organization: formData.organization,
					version: firstVersionFromFile(
						uploadedFile!.hash,
						formData.user_variable_values,
						formData.provisioner_type,
					),
					template: newTemplate(formData),
				});
			}}
		/>
	);
};
