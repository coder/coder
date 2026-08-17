import type { FC } from "react";
import {
	keepPreviousData,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useParams } from "react-router";
import { toast } from "sonner";
import {
	createAndBuildTemplateVersion,
	templateVersion,
	templateVersionsQueryKey,
	updateActiveTemplateVersion,
} from "#/api/queries/templates";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { pageTitle } from "#/utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateDataPageView } from "./TemplateDataPageView";

const TemplateDataPage: FC = () => {
	const { organization = "default" } = useParams<{ organization?: string }>();
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
	const createAndBuildMutation = useMutation(
		createAndBuildTemplateVersion(organization),
	);
	const promoteMutation = useMutation(
		updateActiveTemplateVersion(template, organization, queryClient),
	);

	if (activeVersionError) {
		return <ErrorAlert error={activeVersionError} />;
	}

	if (isActiveVersionLoading || !activeVersion) {
		return <Loader />;
	}

	return (
		<>
			<title>{pageTitle(template.name, "Data")}</title>

			<TemplateDataPageView
				activeVersion={activeVersion}
				canRefresh={permissions.canUpdateTemplate}
				isRefreshing={
					createAndBuildMutation.isPending || promoteMutation.isPending
				}
				error={createAndBuildMutation.error ?? promoteMutation.error}
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
					} catch {
						// Whichever mutation failed records the error, and the view
						// renders it above the refresh button. Letting the rejection
						// escape would only surface it a second time, as an unhandled
						// rejection.
					}
				}}
			/>
		</>
	);
};

export default TemplateDataPage;
