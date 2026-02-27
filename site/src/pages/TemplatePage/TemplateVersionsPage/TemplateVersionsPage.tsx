import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import {
	templateVersions,
	templateVersionsQueryKey,
} from "api/queries/templates";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { linkToTemplate, useLinks } from "modules/navigation";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getTemplatePageTitle } from "../utils";
import { VersionsTable } from "./VersionsTable";

const TemplateVersionsPage = () => {
	const navigate = useNavigate();
	const getLink = useLinks();
	const { template, permissions } = useTemplateLayoutContext();
	const queryClient = useQueryClient();
	const templateLink = getLink(
		linkToTemplate(template.organization_name, template.name),
	);
	const { data } = useQuery(templateVersions(template.id));
	// We use this to update the active version in the UI without having to refetch the template
	const [latestActiveVersion, setLatestActiveVersion] = useState(
		template.active_version_id,
	);
	const { mutate: promoteVersion, isPending: isPromoting } = useMutation({
		mutationFn: (templateVersionId: string) => {
			return API.updateActiveTemplateVersion(template.id, {
				id: templateVersionId,
			});
		},
		onSuccess: async () => {
			const versionName = data?.find(
				(v) => v.id === selectedVersionIdToPromote,
			)?.name;
			setLatestActiveVersion(selectedVersionIdToPromote as string);
			setSelectedVersionIdToPromote(undefined);
			toast.success(
				versionName
					? `Version "${versionName}" promoted successfully.`
					: "Version promoted successfully.",
				{
					action: {
						label: "View template",
						onClick: () => navigate(templateLink),
					},
				},
			);
		},
		onError: (error) => {
			const versionName = data?.find(
				(v) => v.id === selectedVersionIdToPromote,
			)?.name;
			toast.error(
				getErrorMessage(
					error,
					versionName
						? `Failed to promote version "${versionName}".`
						: "Failed to promote version.",
				),
				{
					description: getErrorDetail(error),
				},
			);
		},
	});

	const { mutate: archiveVersion, isPending: isArchiving } = useMutation({
		mutationFn: (templateVersionId: string) => {
			return API.archiveTemplateVersion(templateVersionId);
		},
		onSuccess: async (data) => {
			await queryClient.invalidateQueries({
				queryKey: templateVersionsQueryKey(template.id),
			});
			setSelectedVersionIdToArchive(undefined);
			toast.success(`Version "${data.name}" archived successfully.`);
		},
		onError: (error) => {
			const versionName = data?.find(
				(v) => v.id === selectedVersionIdToArchive,
			)?.name;
			toast.error(
				getErrorMessage(
					error,
					versionName
						? `Failed to archive version "${versionName}".`
						: "Failed to archive version.",
				),
				{
					description: getErrorDetail(error),
				},
			);
		},
	});

	const [selectedVersionIdToPromote, setSelectedVersionIdToPromote] = useState<
		string | undefined
	>();
	const [selectedVersionIdToArchive, setSelectedVersionIdToArchive] = useState<
		string | undefined
	>();

	return (
		<>
			<title>{getTemplatePageTitle("Versions", template)}</title>

			<VersionsTable
				versions={data}
				onPromoteClick={
					permissions.canUpdateTemplate
						? setSelectedVersionIdToPromote
						: undefined
				}
				onArchiveClick={
					permissions.canUpdateTemplate
						? setSelectedVersionIdToArchive
						: undefined
				}
				activeVersionId={latestActiveVersion}
			/>
			{/* Promote confirm */}
			<ConfirmDialog
				type="info"
				hideCancel={false}
				open={selectedVersionIdToPromote !== undefined}
				onConfirm={() => {
					promoteVersion(selectedVersionIdToPromote as string);
				}}
				onClose={() => setSelectedVersionIdToPromote(undefined)}
				title="Promote version"
				confirmLoading={isPromoting}
				confirmText="Promote"
				description="Are you sure you want to promote this version? Workspaces will be prompted to “Update” to this version once promoted."
			/>
			{/* Archive Confirm */}
			<ConfirmDialog
				type="info"
				hideCancel={false}
				open={selectedVersionIdToArchive !== undefined}
				onConfirm={() => {
					archiveVersion(selectedVersionIdToArchive as string);
				}}
				onClose={() => setSelectedVersionIdToArchive(undefined)}
				title="Archive version"
				confirmLoading={isArchiving}
				confirmText="Archive"
				description="Are you sure you want to archive this version (this is reversible)? Archived versions cannot be used by workspaces."
			/>
		</>
	);
};

export default TemplateVersionsPage;
