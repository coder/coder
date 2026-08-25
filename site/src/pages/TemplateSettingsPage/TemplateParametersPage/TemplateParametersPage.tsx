import {
	keepPreviousData,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { toast } from "sonner";
import { API } from "#/api/api";
import {
	createAndBuildTemplateVersion,
	enableTemplateParameterCompatibilityMode,
	templateByNameKey,
	templateVersion,
	templateVersionsQueryKey,
	updateActiveTemplateVersion,
} from "#/api/queries/templates";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { pageTitle } from "#/utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateParametersPageView } from "./TemplateParametersPageView";

const TemplateParametersPage: React.FC = () => {
	const { template, permissions } = useTemplateSettings();
	const queryClient = useQueryClient();

	const {
		data: activeVersion,
		error: activeVersionError,
		isLoading: isActiveVersionLoading,
	} = useQuery({
		...templateVersion(template.active_version_id),
		placeholderData: keepPreviousData,
	});
	const compatibililtyModeMutation = useMutation(
		enableTemplateParameterCompatibilityMode(template, queryClient),
	);
	const createAndBuildMutation = useMutation(
		createAndBuildTemplateVersion(template.organization_name),
	);
	const promoteMutation = useMutation(
		updateActiveTemplateVersion(template, queryClient),
	);

	if (activeVersionError) {
		return <ErrorAlert error={activeVersionError} />;
	}

	if (isActiveVersionLoading || !activeVersion) {
		return <Loader />;
	}

	return (
		<>
			<title>{pageTitle(template.name, "Parameters")}</title>

			<TemplateParametersPageView
				activeVersion={activeVersion}
				useClassicParameterFlow={template.use_classic_parameter_flow}
				canUpdate={permissions.canUpdateTemplate}
				isSaving={compatibililtyModeMutation.isPending}
				isRefreshing={
					createAndBuildMutation.isPending || promoteMutation.isPending
				}
				error={
					compatibililtyModeMutation.error ??
					createAndBuildMutation.error ??
					promoteMutation.error
				}
				onChangeClassicParameterFlow={async (useClassicParameterFlow) => {
					compatibililtyModeMutation.reset();

					try {
						await compatibililtyModeMutation.mutateAsync(
							useClassicParameterFlow,
						);
						toast.success(
							useClassicParameterFlow
								? `${template.display_name} will use parameter compatibility mode.`
								: `${template.display_name} will use dynamic parameters.`,
						);
					} catch {}
				}}
				onRefresh={async () => {
					createAndBuildMutation.reset();
					promoteMutation.reset();

					try {
						const newVersion = await createAndBuildMutation.mutateAsync({
							template_id: template.id,
							provisioner: "terraform",
							storage_method: "file",
							file_id: activeVersion.job.file_id,
							tags: activeVersion.job.tags,
							message: "Refreshed template data",
						});
						await queryClient.invalidateQueries({
							queryKey: templateVersionsQueryKey(template.id),
						});
						await promoteMutation.mutateAsync(newVersion.id);
						toast.success(
							`Template "${template.name}" data refreshed successfully.`,
						);
					} catch {}
				}}
			/>
		</>
	);
};

export default TemplateParametersPage;
