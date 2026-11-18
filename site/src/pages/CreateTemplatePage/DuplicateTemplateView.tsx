import { useQuery } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { checkAuthorization } from "#/api/queries/authCheck";
import {
	JobError,
	template,
	templateVersion,
	templateVersionLogs,
	templateVersionPresets,
	templateVersionVariables,
} from "#/api/queries/templates";
import type { Template, TemplateVersion } from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { templatePermissionChecks } from "#/modules/permissions/templates";
import { CreateTemplateForm } from "./CreateTemplateForm";
import type { CreateTemplatePageViewProps } from "./types";
import { firstVersionFromFile, getFormPermissions, newTemplate } from "./utils";

export const DuplicateTemplateView: React.FC<CreateTemplatePageViewProps> = ({
	onCreateTemplate,
	onOpenBuildLogsDrawer,
	variablesSectionRef,
	error,
	isCreating,
}) => {
	const navigate = useNavigate();
	const { entitlements, showOrganizations } = useDashboard();
	const [searchParams] = useSearchParams();
	const templateQuery = useQuery(
		template(searchParams.get("fromTemplate") as string),
	);
	const permissionsQuery = useQuery({
		...checkAuthorization({
			checks: templatePermissionChecks(templateQuery.data?.id ?? ""),
		}),
		enabled: templateQuery.isSuccess,
	});
	const canUpdateTemplate = permissionsQuery.data?.canUpdateTemplate ?? false;
	const activeVersionId = templateQuery.data?.active_version_id ?? "";
	const templateVersionQuery = useQuery({
		...templateVersion(activeVersionId),
		enabled: canUpdateTemplate,
	});
	const templateVersionVariablesQuery = useQuery({
		...templateVersionVariables(activeVersionId),
		enabled: canUpdateTemplate,
	});
	const { workspace_prebuilds: prebuildsEnabled } = useFeatureVisibility();
	const presetsQuery = useQuery({
		...templateVersionPresets(activeVersionId),
		enabled: canUpdateTemplate && prebuildsEnabled,
	});
	const totalPrebuilds =
		presetsQuery.data?.reduce(
			(sum, preset) => sum + (preset.DesiredPrebuildInstances ?? 0),
			0,
		) ?? 0;
	const isLoading =
		templateQuery.isLoading ||
		permissionsQuery.isLoading ||
		templateVersionQuery.isLoading ||
		templateVersionVariablesQuery.isLoading;
	const loadingError =
		templateQuery.error ||
		permissionsQuery.error ||
		templateVersionQuery.error ||
		templateVersionVariablesQuery.error;

	const formPermissions = getFormPermissions(entitlements);

	const isJobError = error instanceof JobError;
	const templateVersionLogsQuery = useQuery({
		...templateVersionLogs(isJobError ? error.version.id : ""),
		enabled: isJobError,
	});

	if (isLoading) {
		return <Loader />;
	}

	if (
		loadingError &&
		(!templateQuery.data ||
			!permissionsQuery.data ||
			!templateVersionQuery.data ||
			!templateVersionVariablesQuery.data)
	) {
		return <ErrorAlert error={loadingError} />;
	}

	return (
		<RequirePermission isFeatureVisible={canUpdateTemplate}>
			{Boolean(loadingError) && <ErrorAlert error={loadingError} />}
			{totalPrebuilds > 0 && (
				<Alert severity="warning" className="mb-4">
					This template has prebuilds configured. Duplicating this template will
					automatically cause {totalPrebuilds}{" "}
					{totalPrebuilds === 1 ? "prebuild" : "prebuilds"} to be created.
				</Alert>
			)}
			<CreateTemplateForm
				showOrganizationPicker={showOrganizations}
				{...formPermissions}
				variablesSectionRef={variablesSectionRef}
				onOpenBuildLogsDrawer={onOpenBuildLogsDrawer}
				copiedTemplate={templateQuery.data as Template}
				error={error}
				isSubmitting={isCreating}
				variables={templateVersionVariablesQuery.data}
				onCancel={() => navigate(-1)}
				jobError={isJobError ? error.job.error : undefined}
				logs={templateVersionLogsQuery.data}
				onSubmit={async (formData) => {
					await onCreateTemplate({
						organization: formData.organization,
						version: firstVersionFromFile(
							(templateVersionQuery.data as TemplateVersion).job.file_id,
							formData.user_variable_values,
							formData.provisioner_type,
							formData.tags,
						),
						template: newTemplate(formData),
					});
				}}
			/>
		</RequirePermission>
	);
};
